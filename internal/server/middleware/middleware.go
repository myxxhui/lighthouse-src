// Package middleware provides HTTP middleware for the Lighthouse API server.
package middleware

import (
	"math/rand"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestID adds a unique request ID to each request.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.Request.Header.Get("X-Request-Id")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Set("requestId", requestID)
		c.Writer.Header().Set("X-Request-Id", requestID)
		c.Next()
	}
}

// Logger logs HTTP requests.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		end := time.Now()
		latency := end.Sub(start)
		status := c.Writer.Status()
		method := c.Request.Method
		clientIP := c.ClientIP()
		requestID, _ := c.Get("requestId")

		if status >= 400 {
			// Log error requests with additional details
			_, _ = gin.DefaultWriter.Write([]byte(
				formatLog(time.Now(), status, latency, clientIP, method, path, query, requestID, c.Errors.String()),
			))
		} else {
			// Standard log format
			_, _ = gin.DefaultWriter.Write([]byte(
				formatLog(time.Now(), status, latency, clientIP, method, path, query, requestID, ""),
			))
		}
	}
}

// formatLog formats a log entry.
func formatLog(timestamp time.Time, status int, latency time.Duration, clientIP, method, path, query string, requestID interface{}, errors string) string {
	base := timestamp.Format("2006/01/02 - 15:04:05") +
		" | " + clientIP +
		" | " + method +
		" | " + path
	if query != "" {
		base += "?" + query
	}
	base += " | " + string(rune(status)) +
		" | " + latency.String() +
		" | " + requestID.(string)
	if errors != "" {
		base += " | " + errors
	}
	return base + "\n"
}

// Recovery recovers from panics and returns a 500 error.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error":   "Internal Server Error",
					"code":    "INTERNAL_ERROR",
					"message": "An unexpected error occurred",
				})
			}
		}()
		c.Next()
	}
}

// CORS handles Cross-Origin Resource Sharing.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// RateLimiter limits requests per IP (simplified version).
func RateLimiter(maxRequests int, window time.Duration) gin.HandlerFunc {
	// In a real implementation, you would use a token bucket or sliding window
	// For simplicity, we'll use a dummy implementation
	return func(c *gin.Context) {
		// Simulate rate limiting: allow all requests for now
		c.Next()
	}
}

// Auth simulates authentication middleware.
func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// In a real implementation, validate JWT or API key
		// For mock purposes, just set a user context
		c.Set("userId", "mock-user-123")
		c.Set("userRole", "admin")
		c.Next()
	}
}

// RequestTimeout sets a timeout for the request.
func RequestTimeout(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Gin already supports timeout via context, but we can add custom handling
		c.Next()
	}
}

// RandomDelay adds random delay for testing (optional).
func RandomDelay(minDelay, maxDelay int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if minDelay > 0 && maxDelay > minDelay {
			delay := rand.Intn(maxDelay-minDelay) + minDelay
			time.Sleep(time.Duration(delay) * time.Millisecond)
		}
		c.Next()
	}
}
