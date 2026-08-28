package api

import (
	"backend/cluster"
	"backend/migrations"
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
