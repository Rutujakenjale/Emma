package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"coupon-import/internal/handler"
	"coupon-import/internal/service"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/glebarez/sqlite"
)

func main() {
	// Choose service implementation based on env
	var svc service.ImportServiceInterface
	if os.Getenv("USE_DB") == "1" {
		dbPath := os.Getenv("DB_PATH")
		if dbPath == "" {
			dbPath = "data.db"
		}
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			log.Fatalf("open db: %v", err)
		}

		// sqlite driver works best with a single writer connection; limit open
		// connections to avoid "database is locked" under contention.
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		// Configure SQLite for better concurrency: enable WAL and set busy timeout
		if _, err := db.Exec("PRAGMA journal_mode = WAL;"); err != nil {
			log.Printf("warning: set journal_mode WAL: %v", err)
		}
		if _, err := db.Exec("PRAGMA busy_timeout = 5000;"); err != nil {
			log.Printf("warning: set busy_timeout: %v", err)
		}

		// recommended SQLite pragmas: use WAL to allow concurrent reads during writes,
		// and set a busy timeout so concurrent writers wait instead of failing immediately.
		if _, err := db.Exec("PRAGMA journal_mode = WAL;"); err != nil {
			log.Printf("warning: set journal_mode WAL: %v", err)
		}
		if _, err := db.Exec("PRAGMA busy_timeout = 5000;"); err != nil {
			log.Printf("warning: set busy_timeout: %v", err)
		}
		// run migrations
		migrPath := filepath.Join("migrations", "001_init.sql")
		if b, err := os.ReadFile(migrPath); err == nil {
			if _, err := db.Exec(string(b)); err != nil {
				log.Printf("migration exec warning: %v", err)
			}
		} else {
			log.Printf("migration file not found: %v", err)
		}
		svc = service.NewImportService(db)
	} else {
		svc = service.NewInMemoryImportService()
	}

	r := gin.Default()

	// enable CORS so the Swagger UI (served from the same host) can call the API
	r.Use(cors.Default())

	// compute project-relative paths based on this source file location so
	// files are served correctly regardless of the current working dir.
	_, thisFile, _, _ := runtime.Caller(0)
	srcDir := filepath.Dir(thisFile) // cmd/api
	projectRoot := filepath.Clean(filepath.Join(srcDir, "..", ".."))
	specPath := filepath.Join(projectRoot, "openapi.yaml")
	swaggerPath := filepath.Join(projectRoot, "docs", "swagger.html")

	// serve OpenAPI spec
	r.StaticFile("/openapi.yaml", specPath)

	// serve Swagger UI static file and redirect /docs -> /docs/swagger.html
	r.StaticFile("/docs/swagger.html", swaggerPath)
	r.GET("/docs", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/docs/swagger.html")
	})

	api := r.Group("/api/v1")
	{
		api.POST("/imports", handler.NewImportHandler(svc).CreateImport)
		api.GET("/imports/:id", handler.NewImportHandler(svc).GetImport)
		api.GET("/imports/:id/errors", handler.NewImportHandler(svc).GetErrors)
		api.POST("/imports/:id/retry", handler.NewImportHandler(svc).RetryFailed)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "9090"
	}
	addr := ":" + port
	log.Printf("listening %s", addr)
	if err := r.Run(addr); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}
