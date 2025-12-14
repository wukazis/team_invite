package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"team-invite/internal/auth"
	"team-invite/internal/cache"
	"team-invite/internal/config"
	"team-invite/internal/database"
	"team-invite/internal/handlers"
	"team-invite/internal/oauth"
	"team-invite/internal/scheduler"
	adminsvc "team-invite/internal/services/admin"
	"team-invite/internal/services/draw"
	invitesvc "team-invite/internal/services/invite"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := database.New(ctx, cfg.PostgresURL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	jwtMgr := auth.NewManager(cfg.JWTSecret, cfg.JWTIssuer)
	oauthClient := oauth.NewClient(cfg.OAuth)
	prizeCache := cache.NewPrizeConfigCache(cfg.PrizeConfigTTL, store.LoadPrizeConfig)
	envSvc := adminsvc.NewEnvService(cfg.EnvFilePath)
	drawSvc := draw.NewService(store, prizeCache, logger)
	inviter := invitesvc.New(invitesvc.Config{
		Accounts: cfg.InviteAccounts,
		Strategy: cfg.InviteStrategy,
		ActiveID: cfg.InviteActiveID,
	})

	quotaRunner := scheduler.NewQuotaRunner(store, cfg.QuotaScheduleTick, logger)
	quotaRunner.Start(ctx)
	defer quotaRunner.Stop()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	handler := handlers.NewHandler(cfg, store, drawSvc, envSvc, jwtMgr, oauthClient, prizeCache, inviter, logger)
	handlers.RegisterRoutes(r, handler, jwtMgr, cfg.AdminAllowedIPs)

	srv := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: r,
	}

	go func() {
		logger.Info("server starting", "port", cfg.HTTPPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err)
	}
	logger.Info("server stopped")
}
