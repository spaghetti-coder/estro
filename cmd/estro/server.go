package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spaghetti-coder/estro/internal/app"
)

func runServer() int {
	flags := parseServerFlags()

	if flags.configPath == "" {
		slog.Error("config file path required: use -config flag or ESTRO_CONFIG env var")
		return 1
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := app.Run(ctx, app.Config{
		ConfigPath: flags.configPath,
		StaticDir:  flags.staticDir,
	}); err != nil {
		slog.Error("server stopped", "error", err)
		return 1
	}

	return 0
}
