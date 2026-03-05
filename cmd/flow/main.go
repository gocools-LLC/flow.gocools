package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	internalaws "github.com/gocools-LLC/flow.gocools/internal/aws"
	"github.com/gocools-LLC/flow.gocools/internal/httpserver"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("flow exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	addr := envOrDefault("FLOW_HTTP_ADDR", ":8080")
	awsRuntimeConfig := internalaws.RuntimeConfigFromEnv()

	logger.Info(
		"aws_auth_configuration",
		"region", awsRuntimeConfig.Session.Region,
		"role_arn_set", awsRuntimeConfig.Session.RoleARN != "",
		"session_name", awsRuntimeConfig.Session.SessionName,
		"external_id_set", awsRuntimeConfig.Session.ExternalID != "",
		"validate_on_start", awsRuntimeConfig.ValidateOnStart,
	)

	if awsRuntimeConfig.ValidateOnStart {
		validateCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := internalaws.ValidateCredentials(validateCtx, awsRuntimeConfig.Session); err != nil {
			return err
		}
		logger.Info("aws_credentials_validation_succeeded")
	}

	srv := httpserver.New(httpserver.Config{
		Addr:    addr,
		Version: version,
		Logger:  logger,
	})

	serverErrCh := make(chan error, 1)
	go func() {
		logger.Info("starting flow service", "addr", addr, "version", version)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
		}
	}()

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErrCh:
		return err
	case <-sigCtx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	logger.Info("flow service stopped")
	return nil
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
