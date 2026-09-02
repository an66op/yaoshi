// Command drawfeed runs the official draw synchronizer and its read-only API
// as a standalone service. It is ready to back a dedicated results website
// without exposing admin, account, or betting endpoints.
package main

import (
	"backend/api"
	"backend/cluster"
	"backend/config"
	"backend/lotteryfeed"
	"backend/services"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	config.LoadConfig()
	cfg := config.GetConfig()
	rootContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := cluster.Init(rootContext, cluster.Options{
		Addr: cfg.Redis.Addr, Username: cfg.Redis.Username, Password: cfg.Redis.Password, DB: cfg.Redis.DB,
		TLS: cfg.Redis.TLS, Prefix: cfg.Redis.Prefix, Required: cfg.Server.Mode == "release",
	}); err != nil {
		log.Fatalf("初始化 Redis 共享运行时失败: %v", err)
	}
	defer func() {
		if err := cluster.Close(); err != nil {
			log.Printf("关闭 Redis 连接失败: %v", err)
		}
	}()
	db, err := config.ConnectDB()
	if err != nil {
		log.Fatal(err)
	}
	if err := services.SeedLotteryCatalog(db, services.LotterySeedOptions{}); err != nil {
		log.Fatalf("初始化开奖数据失败: %v", err)
	}

	lotteryService := services.NewLotteryService(db)
	scheduler := lotteryfeed.NewScheduler(lotteryfeed.DefaultJobs(), func(ctx context.Context, group string) []lotteryfeed.SyncResult {
		results := lotteryService.SyncOfficialGroup(ctx, group)
		mapped := make([]lotteryfeed.SyncResult, 0, len(results))
		for _, result := range results {
			mapped = append(mapped, lotteryfeed.SyncResult{GameID: result.GameID, Status: result.Status, Imported: result.Imported, LatestIssue: result.LatestIssue, Error: result.Error})
		}
		return mapped
	})
	scheduler.Start(rootContext)
	services.StartSettlementRecovery(rootContext, db)

	r := gin.New()
	r.Use(api.SafeRequestLogger(), gin.Recovery(), api.SecurityHeaders(), api.RequestBodyLimit(), api.Cors())
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "drawfeed"}) })
	api.LoadDrawFeedRoutes(r, db, scheduler)

	port := envInt("DRAWFEED_PORT", 8081)
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       75 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	go func() {
		log.Printf("开奖数据服务已启动，监听端口 %d", port)
		if serveErr := server.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Fatal(serveErr)
		}
	}()
	<-rootContext.Done()
	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		log.Printf("开奖数据服务关闭超时: %v", err)
	}
}

func envInt(name string, fallback int) int {
	if value := os.Getenv(name); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}
