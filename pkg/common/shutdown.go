package common

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func GracefulShutdown(ctx context.Context, logger *slog.Logger, cleanup ...func(ctx context.Context)) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
	case sig := <-sigCh:
		logger.Info("Received signal, shutting down", "signal", sig)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for i := len(cleanup) - 1; i >= 0; i-- {
		if cleanup[i] != nil {
			cleanup[i](shutdownCtx)
		}
	}
}
