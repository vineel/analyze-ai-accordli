package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"workos-prototype/internal/auth"
)

// Me returns the active session info to the frontend.
func Me(c *gin.Context) {
	s := auth.GetSession(c)
	if s == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no session"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user_id":         s.UserID,
		"email":           s.Email,
		"first_name":      s.FirstName,
		"last_name":       s.LastName,
		"organization_id": s.OrganizationID,
		"role":            s.Role,
		"expires_at":      s.ExpiresAt,
	})
}
