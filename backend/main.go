package main

import (
	"backend/api"
	"backend/config"
	"backend/constants"
	"backend/data/models/user"
	"backend/lotteryfeed"
	"backend/services"
	"backend/utils"
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	// 初始化配置
	config.LoadConfig()
	cfg := config.GetConfig()

	// 初始化JWT
	utils.InitJWT(cfg.JWT.Secret)

	// 连接数据库
	db, err := config.ConnectDB()
	if err != nil {
		log.Fatalf("%s: %v", constants.ErrDatabaseConnectionFailed, err)
	}
	// 初始化依赖
	if err := InitDependencies(db); err != nil {
		log.Fatalf("%s: %v", constants.ErrInitDependenciesFailed, err)
	}

	rootContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
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

	r := gin.Default()
	if err := r.SetTrustedProxies(cfg.Server.TrustedProxies); err != nil {
		log.Fatalf("受信任代理配置错误: %v", err)
	}
	r.Use(api.Cors())
	// 加载路由
	api.LoadRoutes(r, db, scheduler)
	log.Printf(constants.ServerStartMessage, cfg.Server.Port)
	server := &http.Server{Addr: fmt.Sprintf(":%d", cfg.Server.Port), Handler: r, ReadHeaderTimeout: 10 * time.Second}
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

// InitDependencies 初始化依赖
func InitDependencies(db *gorm.DB) error {
	// 初始化管理员用户
	userService := services.NewUserService(db)
	_, err := userService.GetUserByUsername(constants.DefaultAdminUsername)
	if err != nil {
		// 不是"用户未找到"的情况，直接返回错误
		if !errors.Is(err, gorm.ErrRecordNotFound) && !strings.Contains(err.Error(), constants.ErrUserNotFound) {
			return err
		}

		// 用户不存在时，创建一个默认管理员
		hashedPwd, err := utils.HashPassword(constants.DefaultAdminPassword)
		if err != nil {
			return fmt.Errorf("%s: %w", constants.ErrCreateAdminPasswordFailed, err)
		}

		admin := &user.User{
			Username: constants.DefaultAdminUsername,
			Password: hashedPwd,
			Nickname: constants.DefaultAdminNickname,
			Email:    constants.DefaultAdminEmail,
			Role:     "admin",
			Status:   1,
		}
		if err := userService.CreateUser(admin); err != nil {
			return fmt.Errorf("%s: %w", constants.ErrCreateAdminUserFailed, err)
		}
	} else {
		// Keep the bootstrap account marked as admin so the new auth gate works
		// on databases that were seeded before Role was introduced.
		_ = db.Model(&user.User{}).Where("username = ? AND (role IS NULL OR role = '' OR role <> ?)", constants.DefaultAdminUsername, "admin").Update("role", "admin").Error
	}

	if err := services.SeedLotteryData(db); err != nil {
		return fmt.Errorf("初始化开奖数据失败: %w", err)
	}

	return nil
}
