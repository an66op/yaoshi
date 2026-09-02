package services

import (
	"backend/data/models/activity"
	"backend/data/models/application"
	"backend/data/models/audit"
	"backend/data/models/bet"
	"backend/data/models/chat"
	"backend/data/models/lottery"
	"backend/data/models/notify"
	"backend/data/models/plan"
	"backend/data/models/profitshare"
	"backend/data/models/rebate"
	modeluser "backend/data/models/user"
	"backend/data/models/wallet"
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gorm.io/gorm"
)

// This allow-list turns every new GORM Delete call into a conscious policy
// decision during review. Counts are intentional: replacing a row set in a
// transaction is different from adding a new destructive endpoint.
func TestGORMDeleteCallsMatchAuditedAllowList(t *testing.T) {
	type allowance struct {
		count  int
		reason string
	}
	want := map[string]allowance{
		"services/activity_admin.go:&activity.Activity{}":    {1, "soft-deleted room activity"},
		"services/lottery.go:&category":                      {1, "soft-deleted lobby category"},
		"services/member_payment_account.go:&row":            {1, "soft-deleted member receiving account"},
		"services/notify_admin.go:&notify.Notification{}":    {3, "soft-deleted derived admin notices"},
		"services/plan_content.go:&plan.Recommendation{}":    {1, "soft-deleted room plan recommendation"},
		"services/plan_retention.go:&plan.StreamPeriod{}":    {1, "explicit stream-scoped derived period retention: keep newest 20; current-cycle high-water is retained"},
		"services/plan_retention.go:&plan.StreamCycle{}":     {1, "explicit stream-scoped derived cycle payload cleanup, only unreferenced noncurrent cycles"},
		"services/odds_admin.go:&odds.PlayLimit{}":           {2, "transactional replace/reset of reconstructable limits"},
		"services/odds_admin.go:&odds.RoomPlayOdds{}":        {2, "transactional invalidation of room overrides when platform odds are disabled or reset"},
		"services/odds_admin.go:&odds.UserPlayOdds{}":        {2, "transactional invalidation of member overrides when platform odds are disabled or reset"},
		"services/trading_admin.go:&odds.RoomPlayOdds{}":     {1, "remove an inherited room override"},
		"services/trading_admin.go:&odds.UserPlayOdds{}":     {1, "remove an inherited member override"},
		"services/user_admin.go:&workspacemodel.RobotGame{}": {1, "transactional replacement of robot-game assignments"},
		"services/wallet_admin.go:&wallet.PaymentChannel{}":  {1, "soft-deleted room payment channel"},
	}

	root := backendRoot(t)
	servicesDir := filepath.Join(root, "services")
	got := map[string]int{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(servicesDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		rel, _ := filepath.Rel(root, path)
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Delete" || len(call.Args) == 0 {
				return true
			}
			if expressionCallsMethod(selector.X, "Unscoped") {
				t.Errorf("Unscoped().Delete is forbidden in %s", rel)
			}
			var rendered bytes.Buffer
			if err := format.Node(&rendered, fset, call.Args[0]); err != nil {
				t.Errorf("format Delete argument in %s: %v", rel, err)
				return true
			}
			key := filepath.ToSlash(rel) + ":" + rendered.String()
			got[key]++
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan service Delete calls: %v", err)
	}

	keys := make([]string, 0, len(want)+len(got))
	seen := map[string]bool{}
	for key := range want {
		keys = append(keys, key)
		seen[key] = true
	}
	for key := range got {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		allowed, exists := want[key]
		if !exists {
			t.Errorf("unaudited GORM Delete call %s (count=%d)", key, got[key])
			continue
		}
		if got[key] != allowed.count {
			t.Errorf("Delete call %s count=%d, want=%d (%s)", key, got[key], allowed.count, allowed.reason)
		}
	}
}

func TestNoRawOrUnscopedDeleteOutsideLifecycle(t *testing.T) {
	root := backendRoot(t)
	rawDelete := regexp.MustCompile(`(?is)\bDELETE\s+FROM\b`)
	truncateTable := regexp.MustCompile(`(?is)\bTRUNCATE\s+(?:TABLE\s+)?["A-Za-z_]`)

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if filepath.ToSlash(rel) != "services/data_lifecycle.go" && rawDelete.Match(contents) {
			t.Errorf("raw DELETE FROM is only allowed in verified lifecycle code: %s", rel)
		}
		if truncateTable.Match(contents) {
			t.Errorf("TRUNCATE is forbidden in application code: %s", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan destructive SQL: %v", err)
	}
}

func expressionCallsMethod(expression ast.Expr, method string) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok && selector.Sel.Name == method {
			found = true
			return false
		}
		return true
	})
	return found
}

func TestDeletionPolicyModelBoundaries(t *testing.T) {
	deletedAtType := reflect.TypeOf(gorm.DeletedAt{})
	for name, model := range map[string]any{
		"user":                   modeluser.User{},
		"activity":               activity.Activity{},
		"payment_channel":        wallet.PaymentChannel{},
		"member_payment_account": wallet.MemberPaymentAccount{},
		"lobby_category":         lottery.LobbyCategory{},
		"chat_message":           chat.Message{},
		"member_notification":    notify.MemberNotification{},
		"admin_notification":     notify.Notification{},
		"plan_recommendation":    plan.Recommendation{},
	} {
		field, ok := reflect.TypeOf(model).FieldByName("DeletedAt")
		if !ok || field.Type != deletedAtType {
			t.Errorf("recoverable model %s DeletedAt=%v, want gorm.DeletedAt", name, field.Type)
		}
	}

	for name, model := range map[string]any{
		"bet":                    bet.Bet{},
		"balance_transaction":    modeluser.BalanceTransaction{},
		"application":            application.Application{},
		"draw":                   lottery.Draw{},
		"issue":                  lottery.Issue{},
		"activity_participation": activity.Participation{},
		"red_packet":             chat.RedPacket{},
		"red_packet_claim":       chat.RedPacketClaim{},
		"rebate":                 rebate.DailyRecord{},
		"profit_share":           profitshare.DailyRecord{},
		"audit_log":              audit.Log{},
	} {
		if _, ok := reflect.TypeOf(model).FieldByName("DeletedAt"); ok {
			t.Errorf("immutable model %s must not be hidden by ordinary soft deletion", name)
		}
	}
}

func TestRobotChatLifecycleIsSelectiveAndReversible(t *testing.T) {
	spec, ok := lifecycleSpecs["robot_chat_messages"]
	if !ok {
		t.Fatal("robot_chat_messages lifecycle class is missing")
	}
	if spec.Action != "soft_delete" || spec.DefaultDays != 30 || spec.MinimumDays != 1 {
		t.Fatalf("unexpected robot chat lifecycle spec: %#v", spec)
	}
}

func backendRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller is unavailable")
	}
	return filepath.Dir(filepath.Dir(filename))
}
