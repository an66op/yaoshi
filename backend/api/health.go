package api

import (
	"backend/cluster"
	"backend/migrations"
	"backend/services"
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// readinessHandler verifies that the process can reach PostgreSQL and that
// every schema migration required by this release has been committed. The
// liveness endpoint intentionally stays lightweight.
func readinessHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		sqlDB, err := db.DB()
		if err != nil || sqlDB.PingContext(c.Request.Context()) != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "reason": "database_unavailable"})
			return
		}

		if err = migrations.VerifyApplied(db.WithContext(c.Request.Context())); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "reason": "schema_incomplete"})
			return
		}
		if cluster.Required() {
			redisClient := cluster.Client()
			if redisClient == nil || redisClient.Ping(c.Request.Context()).Err() != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "reason": "redis_unavailable"})
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	}
}

type productionOddsAudit func(context.Context, *gorm.DB) (*services.ProductionOddsReadinessReport, error)
type sensitiveFieldAudit func(context.Context, *gorm.DB) (*services.SensitiveFieldReadinessReport, error)

// oddsReadinessHandler is separate from process readiness so an operator can
// start a freshly migrated release and configure prices. The formal production
// readiness command calls this read-only gate before traffic is admitted.
func oddsReadinessHandler(db *gorm.DB) gin.HandlerFunc {
	return oddsReadinessHandlerWithAudit(db, services.AuditProductionOddsReadiness)
}

func oddsReadinessHandlerWithAudit(db *gorm.DB, audit productionOddsAudit) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if audit == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "reason": "odds_audit_unavailable"})
			return
		}
		report, err := audit(c.Request.Context(), db)
		if err != nil || report == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "reason": "odds_audit_unavailable"})
			return
		}
		if !report.Complete {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "reason": "odds_incomplete", "odds": report})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready", "odds": report})
	}
}

// sensitiveFieldReadinessHandler authenticates every persisted encrypted
// field and returns aggregate counts only. It is separate from the frequent
// process probe because a complete inventory intentionally scans all rows.
func sensitiveFieldReadinessHandler(db *gorm.DB) gin.HandlerFunc {
	return sensitiveFieldReadinessHandlerWithAudit(db, services.AuditSensitiveFieldReadiness)
}

func sensitiveFieldReadinessHandlerWithAudit(db *gorm.DB, audit sensitiveFieldAudit) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if audit == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "reason": "encryption_inventory_unavailable"})
			return
		}
		report, err := audit(c.Request.Context(), db)
		if err != nil || report == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "reason": "encryption_inventory_unavailable"})
			return
		}
		if !report.Complete {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "reason": "encryption_incomplete", "encryption": report})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready", "encryption": report})
	}
}
