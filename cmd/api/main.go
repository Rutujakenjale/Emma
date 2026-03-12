package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"coupon-import/internal/handler"
	"coupon-import/internal/service"

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

	// serve OpenAPI spec
	r.GET("/openapi.yaml", func(c *gin.Context) {
		c.File("openapi.yaml")
	})

	// serve Swagger UI at /docs
	r.GET("/docs", func(c *gin.Context) {
		c.File("docs/swagger.html")
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
