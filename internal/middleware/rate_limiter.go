package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	maxRequests = 30
	window      = time.Minute
)

type visitor struct {
	count    int
	lastSeen time.Time
}

var (
	cleanOnce sync.Once
	mu        sync.Mutex
	visitors  = make(map[string]*visitor)
)

func RateLimiter() gin.HandlerFunc {
	cleanOnce.Do(func() {
		go func() {
			for {
				time.Sleep(5 * time.Minute)
				mu.Lock()
				for ip, v := range visitors {
					if time.Since(v.lastSeen) > window {
						delete(visitors, ip)
					}
				}
				mu.Unlock()
			}
		}()
	})

	return func(c *gin.Context) {
		ip := c.ClientIP()

		mu.Lock()
		v, ok := visitors[ip]
		if !ok || time.Since(v.lastSeen) > window {
			visitors[ip] = &visitor{count: 1, lastSeen: time.Now()}
			mu.Unlock()
			c.Next()
			return
		}

		if v.count >= maxRequests {
			mu.Unlock()
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "muitas requisições, tente novamente em instantes",
			})
			return
		}

		v.count++
		mu.Unlock()
		c.Next()
	}
}
