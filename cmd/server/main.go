package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	stdhttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/glebmavi/pr_reviewer_service/internal/app"
	"github.com/glebmavi/pr_reviewer_service/internal/http"
	"github.com/glebmavi/pr_reviewer_service/internal/storage/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)
	logger.Info("starting service...")

	err := godotenv.Load()
	if err != nil {
		logger.Warn("Error loading .env file, using environment variables. In prod should be ok.")
	}

	dbURL := os.Getenv("APP_DB_URL")
	if dbURL == "" {
		logger.Error("APP_DB_URL is not set")
		os.Exit(1)
	}

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	dbPool, err := initDB(context.Background(), dbURL)
	if err != nil {
		logger.Error("failed to init db", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer dbPool.Close()
	logger.Info("database connection pool established")

	repository := postgres.NewRepository(dbPool, logger.With("layer", "repository"))

	pullRequestService := app.NewPullRequestService(repository, repository, repository, repository, logger.With("service", "pr"))
	teamService := app.NewTeamService(repository, repository, pullRequestService, repository, logger.With("service", "team"))
	userService := app.NewUserService(repository, repository, pullRequestService, repository, logger.With("service", "user"))
	statsService := app.NewStatsService(repository, logger.With("service", "stats"))

	handler := http.NewHandler(teamService, pullRequestService, userService, statsService, logger.With("layer", "http"))
	router := http.NewRouter(handler)

	server := &stdhttp.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	go func() {
		logger.Info(fmt.Sprintf("server starting on port %s", port))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			logger.Error("server listen error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("server exited gracefully")
}

func initDB(ctx context.Context, dbURL string) (*pgxpool.Pool, error) {
	var pool *pgxpool.Pool
	var err error

	for i := 0; i < 5; i++ {
		pool, err = pgxpool.New(ctx, dbURL)
		if err == nil {
			if err = pool.Ping(ctx); err == nil {
				return pool, nil
			}
		}
		slog.Warn("failed to connect to db, retrying...", "attempt", i+1, "error", err.Error())
		time.Sleep(2 * time.Second)
	}

	return nil, fmt.Errorf("failed to connect to database after 5 attempts: %w", err)
}
