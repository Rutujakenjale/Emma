package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"time"

	"coupon-import/internal/handler"
	"coupon-import/internal/service"
	"coupon-import/internal/worker"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/glebarez/sqlite"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// Choose service implementation based on env
	var svc service.ImportServiceInterface
	var db *sql.DB
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

	// enable CORS so the Swagger UI (served from the same host) can call the API
	r.Use(cors.Default())

	// compute project-relative paths based on this source file location so
	// files are served correctly regardless of the current working dir.
	_, thisFile, _, _ := runtime.Caller(0)
	srcDir := filepath.Dir(thisFile) // cmd/api
	projectRoot := filepath.Clean(filepath.Join(srcDir, "..", ".."))
	specPath := filepath.Join(projectRoot, "openapi.yaml")
	swaggerPath := filepath.Join(projectRoot, "docs", "swagger.html")

	// serve OpenAPI spec and docs; protect with API key middleware when configured
	auth := r.Group("/")
	auth.Use(handler.APIKeyMiddleware())
	auth.StaticFile("/openapi.yaml", specPath)
	// serve Swagger UI static file and redirect /docs -> /docs/swagger.html
	auth.StaticFile("/docs/swagger.html", swaggerPath)
	auth.GET("/docs", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/docs/swagger.html")
	})

	// start a bounded worker pool for import processing
	// capacity: 100 queued jobs, workers: 4
	wp := worker.NewPool(svc, 100, 4)

	api := r.Group("/api/v1")
	{
		h := handler.NewImportHandler(svc, wp)
		api.POST("/imports", h.CreateImport)
		api.GET("/imports/:id", h.GetImport)
		api.GET("/imports/:id/errors", h.GetErrors)
		api.POST("/imports/:id/retry", h.RetryFailed)
	}

	// register health routes and metrics
	handler.RegisterHealthRoutes(r, db)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	port := os.Getenv("PORT")
	if port == "" {
		port = "9090"
	}
	addr := ":" + port
	// run server and handle graceful shutdown
	srvErr := make(chan error, 1)
	go func() {
		log.Printf("listening %s", addr)
		if err := r.Run(addr); err != nil && err != http.ErrServerClosed {
			srvErr <- err
		}
		close(srvErr)
	}()

	// wait for termination signal (SIGINT/SIGTERM)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	select {
	case <-stop:
		log.Printf("shutting down: waiting for workers to finish")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := wp.Shutdown(ctx); err != nil {
			log.Printf("worker shutdown error: %v", err)
		}
	case err := <-srvErr:
		if err != nil {
			log.Fatalf("server error: %v", err)
		}
	}
}
