package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// rateLimitByIP returns a fixed-window rate limiter keyed on client IP.
// reqs is the maximum number of requests allowed within window.
// On Redis failure the middleware fails open (does not block the request).
func rateLimitByIP(rdb *redis.Client, reqs int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		windowStart := time.Now().Truncate(window).Unix()
		key := fmt.Sprintf("rl:%s:%d", c.ClientIP(), windowStart)

		count, err := rdb.Incr(c.Request.Context(), key).Result()
		if err != nil {
			c.Next()
			return
		}
		if count == 1 {
			rdb.Expire(c.Request.Context(), key, window+time.Second)
		}
		if count > int64(reqs) {
			c.Header("Retry-After", fmt.Sprintf("%d", int(window.Seconds())))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
			return
		}
		c.Next()
	}
}
