package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/workos/workos-go/v6/pkg/usermanagement"

	"workos-prototype/internal/auth"
	"workos-prototype/internal/handlers"
)

func main() {
	// Load .env from the repo root (one directory up from be/)
	envPath := filepath.Join("..", ".env")
	if err := godotenv.Load(envPath); err != nil {
		log.Printf("no .env at %s loaded; relying on environment vars", envPath)
	}

	cfg := &auth.Config{
		ClientID:       mustEnv("WORKOS_CLIENT_ID"),
		APIKey:         mustEnv("WORKOS_API_KEY"),
		CookiePassword: mustEnv("WORKOS_COOKIE_PASSWORD"),
		RedirectURI:    envOr("WORKOS_REDIRECT_URI", "http://localhost:8080/auth/callback"),
		FrontendURL:    envOr("FRONTEND_URL", "http://localhost:5173"),
	}

	usermanagement.SetAPIKey(cfg.APIKey)

	r := gin.Default()

	r.GET("/login", handlers.Login(cfg))
	r.GET("/auth/callback", handlers.Callback(cfg))
	r.POST("/logout", handlers.Logout(cfg))

	api := r.Group("/api")
	api.Use(auth.Middleware(cfg))
	api.GET("/me", handlers.Me)
	api.GET("/public", handlers.Public)
	api.GET("/admin-only", auth.RequireRole("admin"), handlers.AdminOnly)

	log.Println("server listening on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("missing required env var %s", k)
	}
	return v
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
