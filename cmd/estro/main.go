package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/spaghetti-coder/estro"
	"github.com/spaghetti-coder/estro/internal/config"
	"github.com/spaghetti-coder/estro/internal/handler"
	"github.com/spaghetti-coder/estro/internal/job"
	appMiddleware "github.com/spaghetti-coder/estro/internal/middleware"
)

const (
	defaultHostname = "127.0.0.1"
	defaultPort     = 3000
)

func main() {
	configPath := flag.String("config", os.Getenv("ESTRO_CONFIG"), "path to config file (or set ESTRO_CONFIG env var)")
	staticDir := flag.String("static-dir", "", "path to static files directory")
	flag.Parse()

	if *configPath == "" {
		slog.Error("config file path required: use -config flag or ESTRO_CONFIG env var")
		os.Exit(1)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	jobStore := job.NewStore()

	cmdCtx, cmdCancel := context.WithCancel(context.Background())

	sessionSecret := handler.GenerateSessionSecret()
	globalCfg := cfg.GetGlobal()
	if globalCfg.Secret != nil {
		sessionSecret = []byte(*globalCfg.Secret)
	}
	sessionStore := handler.NewSessionStore(sessionSecret)

	h := handler.NewHandler(cfg, jobStore, sessionStore, sessionSecret, cmdCtx)

	e := echo.New()
	e.Use(appMiddleware.SecurityMiddleware("default-src 'self'"))
	e.Use(appMiddleware.FaviconCORS())
	e.Use(handler.SessionMiddleware(sessionStore))
	e.Use(middleware.RequestLogger())

	h.RegisterRoutes(e)

	if *staticDir != "" {
		e.Static("/", *staticDir)
	} else {
		subFS, err := fs.Sub(estro.StaticFS, "public")
		if err != nil {
			slog.Error("failed to create sub filesystem", "error", err)
			os.Exit(1)
		}
		e.StaticFS("/", subFS)
	}

	addr := resolveAddr(cfg.GetGlobal())

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		<-ctx.Done()
		slog.Info("shutting down gracefully...")
		cmdCancel()
		jobStore.MarkAllRunningAsError("server shutting down")
	}()

	slog.Info("Estro listening", "address", fmt.Sprintf("http://%s", addr))
	sc := echo.StartConfig{
		Address:         addr,
		GracefulTimeout: 10 * time.Second,
	}
	if err := sc.Start(ctx, e); err != nil {
		slog.Error("server stopped", "error", err)
	}
}

func resolveAddr(global *config.GlobalConfig) string {
	hostname := defaultHostname
	port := defaultPort
	if global.Hostname != nil {
		hostname = *global.Hostname
	}
	if global.Port != nil {
		port = *global.Port
	}
	return fmt.Sprintf("%s:%d", hostname, port)
}
