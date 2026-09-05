package main

import (
	"backend/api"
	"backend/cluster"
	"backend/config"
	"backend/constants"
	"backend/lotteryfeed"
	"backend/middleware"
	"backend/services"
	"backend/utils"
	"backend/ws"
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	// 初始化配置
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
	if cluster.Enabled() {
		log.Printf("Redis 共享运行时已连接，实例 %s", cluster.InstanceID())
	} else {
		log.Printf("Redis 不可用：当前为开发环境单实例回退模式")
	}

	// 初始化JWT
	utils.InitJWT(cfg.JWT.Secret, cfg.JWT.Expire)
	if err := utils.InitFieldEncryptionWithFallbacks(cfg.Security.DataEncryptionKey, cfg.Security.DataEncryptionPreviousKeys); err != nil {
		log.Fatalf("初始化敏感字段加密失败: %v", err)
	}

	// 连接数据库
	db, err := config.ConnectDB()
	if err != nil {
		log.Fatalf("%s: %v", constants.ErrDatabaseConnectionFailed, err)
	}
	// 集中初始化：正式环境不会落入本地账号、模拟历史开奖或计划数据。
	if err := services.Bootstrap(db, services.BootstrapOptions{
		Mode:                            cfg.Server.Mode,
		SeedExperienceAccounts:          cfg.Server.SeedExperienceAccounts,
		SeedDeterministicLotteryHistory: cfg.Server.SeedDeterministicLotteryHistory,
	}); err != nil {
		log.Fatalf("%s: %v", constants.ErrInitDependenciesFailed, err)
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
	scheduler.SetEventSink(services.NewSystemLogService(db).RecordSchedulerEvent)
	gin.SetMode(cfg.Server.Mode)
	r := gin.New()
	if err := r.SetTrustedProxies(cfg.Server.TrustedProxies); err != nil {
		log.Fatalf("受信任代理配置错误: %v", err)
	}
	r.Use(api.SafeRequestLogger(), gin.Recovery(), api.SecurityHeaders(), api.RequestBodyLimit(), api.Cors())
	uploadRoot := strings.TrimSpace(os.Getenv("BACKEND_UPLOAD_DIR"))
	if uploadRoot == "" {
		uploadRoot = "uploads"
	}
	if cfg.Server.Mode == "release" && (!filepath.IsAbs(uploadRoot) || filepath.Clean(uploadRoot) == string(filepath.Separator)) {
		log.Fatalf("release 模式的上传目录必须是非根绝对路径")
	}
	if err := os.MkdirAll(filepath.Join(uploadRoot, "activities"), 0o750); err != nil {
		log.Fatalf("创建上传目录失败: %v", err)
	}
	r.Static("/api/public/uploads", uploadRoot)
	// 加载路由
	api.LoadRoutesForMode(r, db, scheduler, cfg.Server.Mode)
	if err := ws.StartClusterBridge(rootContext, db); err != nil {
		if cluster.Required() {
			log.Fatalf("启动 WebSocket Redis 桥接失败: %v", err)
		}
		log.Printf("WebSocket Redis 桥接不可用，使用本机连接: %v", err)
	}
	scheduler.Start(rootContext)
	services.StartSimulatedDrawLoop(rootContext, db)
	services.StartSettlementRecovery(rootContext, db)
	services.StartSGSSCBackfill(rootContext, db)
	services.StartIdempotencyRecovery(rootContext, db)
	services.StartDataLifecycleLoop(rootContext, db)
	services.StartRoomActivityForMode(rootContext, db, cfg.Server.Mode)
	services.StartRedPacketExpiry(rootContext, db)
	middleware.StartAuditRecovery(rootContext, db)
	log.Printf(constants.ServerStartMessage, cfg.Server.Port)
	server := &http.Server{
		Addr:              net.JoinHostPort(cfg.Server.Bind, fmt.Sprintf("%d", cfg.Server.Port)),
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       75 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	go func() {
		if serveErr := server.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Fatal(constants.ErrServerStartFailed, serveErr)
		}
	}()
	<-rootContext.Done()
	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		log.Printf("服务关闭超时: %v", err)
	}
}
