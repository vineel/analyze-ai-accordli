package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	"github.com/joho/godotenv"

	"accordli.com/analyze-ai/api/internal/httpapi"
	"accordli.com/analyze-ai/api/internal/infra/auth"
	"accordli.com/analyze-ai/api/internal/infra/db"
	"accordli.com/analyze-ai/api/internal/infra/observability"
	"accordli.com/analyze-ai/api/internal/infra/repo"
	"accordli.com/analyze-ai/api/internal/solomocky"
)

func main() {
	seed := flag.Bool("seed", false, "seed the Mocky Org/Dept/User and exit")
	flag.Parse()

	loadEnvFile()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer pool.Close()
	repos := repo.New(pool)

	// Always idempotently seed the Mocky trio so a fresh DB is usable
	// without an extra step. `make seed` and `-seed` short-circuit.
	if err := solomocky.Seed(ctx, repos); err != nil {
		log.Fatalf("seed: %v", err)
	}
	if *seed {
		fmt.Println("seed: ok")
		return
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logger := observability.NewStdout()
	authProvider := auth.NewHardcoded(
		solomocky.UserID, solomocky.OrgID, solomocky.DeptID, solomocky.UserEmail,
	)

	deps := &httpapi.Deps{
		Auth:    authProvider,
		Log:     logger,
		Repos:   repos,
		Version: gitSHA(),
	}
	handler := httpapi.NewRouter(deps)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info(context.Background(), "api listening", map[string]any{
		"addr":    srv.Addr,
		"version": deps.Version,
	})

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
}

// loadEnvFile reads .env at the repo root if present. Real env vars
// already in the process win; .env is only a fallback.
func loadEnvFile() {
	for _, p := range []string{".env", "../.env"} {
		if _, err := os.Stat(p); err == nil {
			_ = godotenv.Load(p)
			return
		}
	}
}

// gitSHA returns the build-stamped VCS revision, or "dev" when running
// via `go run` without -buildvcs.
func gitSHA() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && s.Value != "" {
			if len(s.Value) >= 12 {
				return s.Value[:12]
			}
			return s.Value
		}
	}
	return "dev"
}
