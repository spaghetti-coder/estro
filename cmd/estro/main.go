package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/spaghetti-coder/estro"
	"github.com/spaghetti-coder/estro/internal/auth"
	"github.com/spaghetti-coder/estro/internal/config"
	"github.com/spaghetti-coder/estro/internal/handler"
	"github.com/spaghetti-coder/estro/internal/job"
	appMiddleware "github.com/spaghetti-coder/estro/internal/middleware"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "hash" {
		os.Exit(runHash(os.Args[2:]))
	}

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

	sessionSecret := auth.GenerateSessionSecret()
	globalCfg := cfg.GetGlobal()
	if globalCfg.Secret != nil {
		sessionSecret = []byte(*globalCfg.Secret)
	}
	sessionStore := auth.NewSessionStore(sessionSecret)

	h := handler.NewHandler(cfg, jobStore, sessionStore, sessionSecret, cmdCtx)

	e := echo.New()
	e.Use(appMiddleware.SecurityMiddleware("default-src 'self'"))
	e.Use(appMiddleware.FaviconCORS())
	e.Use(auth.SessionMiddleware(sessionStore))
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

	addr := cfg.GetGlobal().Addr()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		<-ctx.Done()
		slog.Info("shutting down gracefully...")
		cmdCancel()
		jobStore.MarkAllRunningAsError("server shutting down")
	}()

	slog.Info("Estro listening", "address", "http://"+addr)
	sc := echo.StartConfig{
		Address:         addr,
		GracefulTimeout: 10 * time.Second,
	}
	if err := sc.Start(ctx, e); err != nil {
		slog.Error("server stopped", "error", err)
	}
}

func runHash(args []string) int {
	var plainPassword string
	switch len(args) {
	case 0:
		fmt.Fprint(os.Stderr, "Password: ")
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading password: %v\n", err)
			return 1
		}
		plainPassword = string(pw)
	case 1:
		plainPassword = args[0]
	default:
		fmt.Fprintf(os.Stderr, "usage: %s hash [password]\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "  With no argument, prompts for password (no echo).")
		return 1
	}

	prefixed, err := auth.HashPassword(plainPassword)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Println(strings.TrimPrefix(prefixed, "bcrypt:"))
	return 0
}
