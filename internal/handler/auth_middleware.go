package handler

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// APIKeyMiddleware checks for an API key in the X-API-Key header or Authorization Bearer token.
// If the environment variable API_KEY is empty, the middleware is a no-op (allows all requests).
func APIKeyMiddleware() gin.HandlerFunc {
	key := os.Getenv("API_KEY")
	return func(c *gin.Context) {
		if key == "" {
			c.Next()
			return
		}
		if c.GetHeader("X-API-Key") == key {
			c.Next()
			return
		}
		auth := c.GetHeader("Authorization")
		if strings.HasPrefix(auth, "Bearer ") && strings.TrimPrefix(auth, "Bearer ") == key {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "UNAUTHORIZED", "message": "missing or invalid api key"}})
	}
}
