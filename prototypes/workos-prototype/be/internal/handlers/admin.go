package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"workos-prototype/internal/auth"
)

// AdminOnly is gated by RequireRole("admin"). If we got here, the user is an admin.
func AdminOnly(c *gin.Context) {
	s := auth.GetSession(c)
	c.JSON(http.StatusOK, gin.H{
		"message": "admin-only resource — you have access because your role is admin",
		"viewer":  s.Email,
		"role":    s.Role,
	})
}
