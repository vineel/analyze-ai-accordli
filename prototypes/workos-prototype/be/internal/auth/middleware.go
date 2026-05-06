package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const sessionContextKey = "session"

// Middleware extracts and validates the session cookie. Aborts with 401 if absent or invalid.
func Middleware(cfg *Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Cookie(CookieName)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no session"})
			return
		}

		session, err := cfg.Unseal(cookie)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired session"})
			return
		}

		c.Set(sessionContextKey, session)
		c.Next()
	}
}

// RequireRole guards a route — must be chained AFTER Middleware.
func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		s := GetSession(c)
		if s == nil || s.Role != role {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient role"})
			return
		}
		c.Next()
	}
}

// GetSession returns the session attached by Middleware, or nil if there isn't one.
func GetSession(c *gin.Context) *Session {
	v, exists := c.Get(sessionContextKey)
	if !exists {
		return nil
	}
	s, ok := v.(*Session)
	if !ok {
		return nil
	}
	return s
}
