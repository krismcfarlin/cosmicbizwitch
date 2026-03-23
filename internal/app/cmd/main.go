package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"strings"

	"cosmicbizwitch/internal/app/server"
	"cosmicbizwitch/internal/app/storage"
	"cosmicbizwitch/internal/app/triggers"
	appworkflows "cosmicbizwitch/internal/app/workflows"
	"cosmicbizwitch/pkg/workflow"
	"cosmicbizwitch/pkg/workflow/pbstore"

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
	port := getEnvInt("PORT", 8085)

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
			// Ensure dev superuser exists on every startup.
			if _, err := e.App.FindAuthRecordByEmail("_superusers", "krismcfarlin@gmail.com"); err != nil {
				if col, cerr := e.App.FindCollectionByNameOrId("_superusers"); cerr == nil {
					su := core.NewRecord(col)
					su.SetEmail("krismcfarlin@gmail.com")
					su.SetPassword("super1234bad")
					_ = e.App.Save(su)
				}
			}

			logger.Println("Initializing storage...")
			store, err := storage.New(pb, legacyDB, storage.Config{Logger: logger})
			if err != nil {
				return fmt.Errorf("failed to initialize storage: %w", err)
			}

			// Workflow collections
			if err := pbstore.CreateCollections(e.App); err != nil {
				return fmt.Errorf("workflow collections: %w", err)
			}

			// Settings manager — reads CF credentials from app_settings collection.
			settingsMgr := storage.NewSettingsManager(e.App)
			if err := settingsMgr.Reload(logger); err != nil {
				logger.Printf("settings reload warning: %v", err)
			}

			// Workflow engine
			eng := workflow.NewEngine(pbstore.New(e.App), workflow.EngineConfig{Logger: logger})

			// Register default demo activities and graphs
			if err := appworkflows.RegisterDefaults(eng, e.App, settingsMgr.CFClient); err != nil {
				return fmt.Errorf("register default workflows: %w", err)
			}

			// PocketBase hook example — trigger a workflow when a contact is created:
			// e.App.OnRecordAfterCreateSuccess("contacts").BindFunc(func(ev *core.RecordEvent) error {
			//     go eng.CreateWorkflow(context.Background(), "onboard_contact", "onboard_contact",
			//         map[string]any{"contact_id": ev.Record.Id, "email": ev.Record.GetString("email")}, nil)
			//     return ev.Next()
			// })
			eng.Start(context.Background())

			// Trigger manager
			triggerMgr := triggers.New(e.App, eng, logger)
			triggerMgr.Start(context.Background())

			// HTTP server
			logger.Println("Initializing server...")
			srv := server.New(store, server.Config{
				Port:      port,
				Logger:    logger,
				LogBuffer: logBuf,
				Engine:    eng,
				Triggers:  triggerMgr,
				Settings:  settingsMgr,
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

	// Ensure PocketBase serves on our port.
	// If args are just ["serve"] (no --http flag), inject it.
	hasHTTP := false
	for _, a := range os.Args {
		if strings.HasPrefix(a, "--http") {
			hasHTTP = true
			break
		}
	}
	if !hasHTTP {
		// Add serve if missing, then inject --http
		if len(os.Args) == 1 {
			os.Args = append(os.Args, "serve")
		}
		os.Args = append(os.Args, fmt.Sprintf("--http=0.0.0.0:%d", port))
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
