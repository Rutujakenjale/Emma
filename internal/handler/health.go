package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RegisterHealthRoutes registers /healthz and /readyz routes on the router.
func RegisterHealthRoutes(r *gin.Engine, db *sql.DB) {
	r.GET("/healthz", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/readyz", func(c *gin.Context) {
		if db != nil {
			if err := db.Ping(); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"ready": false, "error": err.Error()})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"ready": true})
	})
}
