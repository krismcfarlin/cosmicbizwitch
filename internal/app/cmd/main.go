package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"cosmicbizwitch/internal/app/clickfunnels"
	"cosmicbizwitch/internal/app/server"
	"cosmicbizwitch/internal/app/storage"

	"github.com/joho/godotenv"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	_ "github.com/tursodatabase/go-libsql"
)

func main() {
	_ = godotenv.Load()

	logBuf := server.NewLogBuffer()
	logger := log.New(logBuf.TeeWriter(os.Stdout), "[APP] ", log.LstdFlags)

	dbPath := getEnv("DB_PATH", "data/coaching.db")
	port := getEnvInt("PORT", 8083)

	// ClickFunnels client — optional; only created when CF_API_KEY is set.
	var cfClient *clickfunnels.Client
	if cfAPIKey := os.Getenv("CF_API_KEY"); cfAPIKey != "" {
		cfWorkspaceID := 0
		if raw := os.Getenv("CF_WORKSPACE_ID"); raw != "" {
			if id, err := strconv.Atoi(raw); err == nil {
				cfWorkspaceID = id
			}
		}
		cfClient = clickfunnels.NewClient(clickfunnels.Config{
			APIKey:      cfAPIKey,
			Subdomain:   os.Getenv("CF_SUBDOMAIN"),
			WorkspaceID: cfWorkspaceID,
		})
		logger.Println("ClickFunnels client configured")
	}

	// Resolve paths
	absDBPath, err := filepath.Abs(dbPath)
	if err != nil {
		logger.Fatalf("Failed to resolve DB path: %v", err)
	}
	dataDir := filepath.Dir(absDBPath)

	// Open coaching.db via go-libsql (meetings + vector ops only).
	// storage.New will configure WAL/timeout on it.
	legacyDB, err := sql.Open("libsql", "file:"+absDBPath)
	if err != nil {
		logger.Fatalf("Failed to open coaching.db: %v", err)
	}
	defer legacyDB.Close()

	// PocketBase uses data.db in the same directory as coaching.db.
	pb := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir:  dataDir,
		HideStartBanner: true,
	})

	// After PocketBase bootstraps, initialize store (which creates collections),
	// then wire up routes.
	pb.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
		Func: func(e *core.ServeEvent) error {
			logger.Println("Initializing storage...")
			store, err := storage.New(pb, legacyDB, storage.Config{Logger: logger})
			if err != nil {
				return fmt.Errorf("failed to initialize storage: %w", err)
			}

			// HTTP server
			logger.Println("Initializing server...")
			srv := server.New(store, server.Config{
				Port:      port,
				Logger:    logger,
				LogBuffer: logBuf,
				CFClient:  cfClient,
			})

			// Mount our HTTP handler on PocketBase's router as catch-all.
			// PocketBase's own specific routes (/_/, /api/collections/, etc.) take precedence
			// over this wildcard, so our handler only sees paths PocketBase doesn't own.
			ourHandler := srv.Handler()
			e.Router.Any("/{path...}", func(re *core.RequestEvent) error {
				ourHandler.ServeHTTP(re.Response, re.Request)
				return nil
			})

			logger.Printf("Application started successfully")
			logger.Printf("HTTP server listening on http://localhost:%d", port)
			logger.Printf("  Health:   http://localhost:%d/health", port)
			logger.Printf("  Admin UI: http://localhost:%d/_/", port)
			logger.Printf("  Logs:     http://localhost:%d/logs", port)

			return e.Next()
		},
		Priority: 999,
	})

	// Configure PocketBase to serve on our port
	if len(os.Args) == 1 {
		os.Args = append(os.Args, "serve", fmt.Sprintf("--http=0.0.0.0:%d", port))
	}

	logger.Println("Starting PocketBase...")
	if err := pb.Start(); err != nil {
		logger.Fatal(err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var intValue int
		if _, err := fmt.Sscanf(value, "%d", &intValue); err == nil {
			return intValue
		}
	}
	return defaultValue
}
