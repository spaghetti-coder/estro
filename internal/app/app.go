// Package app assembles and runs the Estro HTTP server.
package app

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/spaghetti-coder/estro"
	"github.com/spaghetti-coder/estro/internal/auth"
	"github.com/spaghetti-coder/estro/internal/config"
	"github.com/spaghetti-coder/estro/internal/handler"
	"github.com/spaghetti-coder/estro/internal/job"
	appMiddleware "github.com/spaghetti-coder/estro/internal/middleware"
)

// Config holds the settings needed to assemble and run the server.
type Config struct {
	ConfigPath string
	StaticDir  string
}

// Option customizes app.Run.
type Option func(*options)

type options struct {
	listener         net.Listener
	listenerAddrFunc func(net.Addr)
}

// WithListener replaces the network listener.
func WithListener(ln net.Listener) Option {
	return func(o *options) {
		o.listener = ln
	}
}

// WithListenerAddrFunc sets a callback invoked with the actual listener address.
func WithListenerAddrFunc(fn func(net.Addr)) Option {
	return func(o *options) {
		o.listenerAddrFunc = fn
	}
}

// Run loads config, builds handlers, middleware and static serving, then
// starts the HTTP server. Runtime server errors are logged but not returned,
// preserving the historical CLI exit-code behavior.
func Run(ctx context.Context, cfg Config, opts ...Option) error {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	res := config.Load(cfg.ConfigPath)
	if !res.Healthy() {
		issues := res.IssueStrings()
		slog.Warn("configuration has issues; starting in degraded mode", "count", len(issues))
		for _, is := range issues {
			slog.Warn("config issue", "issue", is)
		}
	}

	jobStore := job.NewStore()
	cmdCtx, cmdCancel := context.WithCancel(context.Background())
	defer cmdCancel()

	sessionSecret, err := auth.SecretFromConfig(res.Config.GetGlobal())
	if err != nil {
		return fmt.Errorf("resolve session secret: %w", err)
	}
	sessionStore := auth.NewSessionStore(sessionSecret)

	h := handler.NewHandler(res, jobStore, sessionStore, cmdCtx)

	e := echo.New()
	e.Use(appMiddleware.SecurityMiddleware("default-src 'self'"))
	e.Use(appMiddleware.FaviconCORS())
	e.Use(middleware.RequestLogger())

	h.RegisterRoutes(e)

	if err := MountStatic(e, cfg.StaticDir); err != nil {
		return fmt.Errorf("register static files: %w", err)
	}

	go func() {
		<-ctx.Done()
		slog.Info("shutting down gracefully...")
		cmdCancel()
		jobStore.MarkAllRunningAsError("server shutting down")
	}()

	addr := res.ServerAddr()
	slog.Info("Estro listening", "address", "http://"+addr)
	sc := echo.StartConfig{
		Address:          addr,
		GracefulTimeout:  10 * time.Second,
		Listener:         o.listener,
		ListenerAddrFunc: o.listenerAddrFunc,
	}

	if err := sc.Start(ctx, e); err != nil {
		slog.Error("server stopped", "error", err)
	}
	return nil
}

// MountStatic registers static file serving on an Echo instance.
// A non-empty staticDir overrides the embedded public files and must be an
// existing directory.
func MountStatic(e *echo.Echo, staticDir string) error {
	if staticDir != "" {
		info, err := os.Stat(staticDir)
		if err != nil {
			return fmt.Errorf("stat static dir %q: %w", staticDir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("static dir %q is not a directory", staticDir)
		}
		e.Static("/", staticDir)
		return nil
	}

	subFS, err := fs.Sub(estro.StaticFS, "public")
	if err != nil {
		return fmt.Errorf("create sub filesystem: %w", err)
	}
	e.StaticFS("/", subFS)
	return nil
}
