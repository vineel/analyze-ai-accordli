package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"workos-prototype/internal/auth"
)

// Public is readable by any logged-in user, regardless of role.
func Public(c *gin.Context) {
	s := auth.GetSession(c)
	c.JSON(http.StatusOK, gin.H{
		"message": "this is a public resource — any signed-in member can see it",
		"viewer":  s.Email,
		"role":    s.Role,
	})
}
