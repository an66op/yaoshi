package middleware

import (
	"backend/cluster"
	"backend/data/models/audit"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var auditFallbackMu sync.Mutex

func newAuditEventID() string {
	var suffix [12]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return fmt.Sprintf("audit-%d", time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("audit-%d-%s", time.Now().UTC().UnixMilli(), hex.EncodeToString(suffix[:]))
}

func auditFallbackPath() string {
	if configured := strings.TrimSpace(os.Getenv("BACKEND_AUDIT_FALLBACK_FILE")); configured != "" {
		return configured
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("BACKEND_SERVER_MODE")), "release") {
		return "/var/lib/wangzhe/audit-fallback.jsonl"
	}
	return filepath.Join("data", "audit-fallback.jsonl")
}

func persistAuditFallback(entry audit.Log) error {
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	auditFallbackMu.Lock()
	defer auditFallbackMu.Unlock()
	path := auditFallbackPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(payload, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

// StartAuditRecovery replays fsync'd fallback events after PostgreSQL becomes
// available again. EventID has a partial unique index, so replay is safe after
// a crash between the insert and spool compaction.
func StartAuditRecovery(ctx context.Context, db *gorm.DB) {
	if db == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, err := cluster.RunWithLease(ctx, "scheduler:audit-fallback-replay", 10*time.Minute, func() error {
					return replayAuditFallback(db)
				})
				if err != nil && !os.IsNotExist(err) {
					log.Printf("恢复管理审计保底记录失败: %v", err)
				}
			}
		}
	}()
}

func replayAuditFallback(db *gorm.DB) error {
	auditFallbackMu.Lock()
	defer auditFallbackMu.Unlock()
	path := auditFallbackPath()
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(payload), "\n")
	remaining := make([]string, 0)
	latest := make(map[string]audit.Log)
	order := make([]string, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry audit.Log
		if err := json.Unmarshal([]byte(line), &entry); err != nil || strings.TrimSpace(entry.EventID) == "" {
			remaining = append(remaining, line)
			continue
		}
		if _, exists := latest[entry.EventID]; !exists {
			order = append(order, entry.EventID)
		}
		latest[entry.EventID] = entry
	}
	for _, eventID := range order {
		entry := latest[eventID]
		if err := upsertAuditRecord(db, entry); err != nil {
			encoded, marshalErr := json.Marshal(entry)
			if marshalErr == nil {
				remaining = append(remaining, string(encoded))
			}
		}
	}
	contents := ""
	if len(remaining) > 0 {
		contents = strings.Join(remaining, "\n") + "\n"
	}
	return replaceAuditFallbackDurably(path, []byte(contents))
}

// replaceAuditFallbackDurably keeps spool compaction crash-safe. The new
// contents are fsync'd before the atomic rename, then the parent directory is
// fsync'd so the rename itself survives a host crash. Existing file
// permissions are retained rather than being widened by compaction.
func replaceAuditFallbackDurably(path string, contents []byte) (resultErr error) {
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer func() {
		if resultErr != nil {
			_ = os.Remove(temporary)
		}
	}()
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return err
	}
	return directory.Close()
}

func auditRecordUpdates(entry audit.Log) map[string]any {
	return map[string]any{
		"workspace_id": entry.WorkspaceID,
		"actor_id":     entry.ActorID,
		"actor_name":   entry.ActorName,
		"actor_role":   entry.ActorRole,
		"room_scope":   entry.RoomScope,
		"method":       entry.Method,
		"path":         entry.Path,
		"target_ref":   entry.TargetRef,
		"status_code":  entry.StatusCode,
		"request_id":   entry.RequestID,
		"ip":           entry.IP,
	}
}

func updateAuditRecord(db *gorm.DB, entry audit.Log) error {
	return db.Model(&audit.Log{}).Where("event_id = ?", entry.EventID).Updates(auditRecordUpdates(entry)).Error
}

func upsertAuditRecord(db *gorm.DB, entry audit.Log) error {
	entry.ID = 0
	created := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry)
	if created.Error != nil {
		return created.Error
	}
	return updateAuditRecord(db, entry)
}
