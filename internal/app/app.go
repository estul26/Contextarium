// Package app wires the local development process lifecycle.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/estul26/Contextarium/internal/config"
	"github.com/estul26/Contextarium/internal/server"
	"github.com/estul26/Contextarium/internal/storage"
)

func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) (result error) {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	db, err := storage.Open(startupCtx, cfg.DBPath)
	if err != nil {
		logger.Error("database initialization failed")
		return fmt.Errorf("initialize database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("database close failed")
			result = errors.Join(result, err)
		}
	}()
	var lc net.ListenConfig
	listener, err := lc.Listen(startupCtx, "tcp", cfg.ListenAddr)
	if err != nil {
		logger.Error("listener initialization failed")
		return fmt.Errorf("initialize listener: %w", err)
	}
	cancel()
	logger.Info("application ready")
	if err := server.New(db).Serve(ctx, listener); err != nil {
		logger.Error("HTTP serving or shutdown failed")
		return err
	}
	logger.Info("application stopped")
	return nil
}
