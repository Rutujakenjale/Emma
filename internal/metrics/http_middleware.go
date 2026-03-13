package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// GinMiddleware instruments HTTP requests for Prometheus.
func GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		dur := time.Since(start).Seconds()
		HTTPDuration.Observe(dur)
		code := c.Writer.Status()
		method := c.Request.Method
		HTTPRequests.WithLabelValues(method, strconv.Itoa(code)).Inc()
	}
}
