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
	"team-invite/internal/config"
	"team-invite/internal/database"
	"team-invite/internal/handlers"
	"team-invite/internal/oauth"
	adminsvc "team-invite/internal/services/admin"
	invitesvc "team-invite/internal/services/invite"
	"team-invite/internal/services/teamstatus"
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
	envSvc := adminsvc.NewEnvService(cfg.EnvFilePath)
	inviter := invitesvc.New("")
	teamStatusSvc := teamstatus.New(store, logger, 30*time.Second)
	teamStatusSvc.Start(ctx)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	handler := handlers.NewHandler(cfg, store, envSvc, jwtMgr, oauthClient, inviter, teamStatusSvc, logger)
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
