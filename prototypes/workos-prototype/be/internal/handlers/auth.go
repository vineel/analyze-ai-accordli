package handlers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/workos/workos-go/v6/pkg/usermanagement"

	"workos-prototype/internal/auth"
)

// Login generates the AuthKit authorization URL and redirects the user to it.
func Login(cfg *auth.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		url, err := usermanagement.GetAuthorizationURL(usermanagement.GetAuthorizationURLOpts{
			ClientID:    cfg.ClientID,
			RedirectURI: cfg.RedirectURI,
			Provider:    "authkit",
		})
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Redirect(http.StatusFound, url.String())
	}
}

// Callback handles the redirect back from AuthKit. It exchanges the code for a
// user + access token, derives the active org+role, builds a sealed Session
// cookie, and bounces to the frontend.
func Callback(cfg *auth.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		code := c.Query("code")
		if code == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "missing authorization code"})
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		resp, err := usermanagement.AuthenticateWithCode(ctx, usermanagement.AuthenticateWithCodeOpts{
			ClientID: cfg.ClientID,
			Code:     code,
		})
		if err != nil {
			log.Printf("AuthenticateWithCode failed: %v", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		log.Printf("AuthenticateWithCode → user=%s email=%s organization_id=%q method=%s",
			resp.User.ID, resp.User.Email, resp.OrganizationID, resp.AuthenticationMethod)

		// Start by trusting the response's top-level OrganizationID, then enrich
		// with role from the access-token JWT claims.
		orgID := resp.OrganizationID
		role := ""

		if resp.AccessToken != "" {
			claims, perr := auth.ParseAccessToken(resp.AccessToken)
			if perr != nil {
				log.Printf("parse access token: %v", perr)
			} else {
				log.Printf("access token claims → org_id=%q role=%q", claims.OrganizationID, claims.Role)
				if orgID == "" {
					orgID = claims.OrganizationID
				}
				role = claims.Role
			}
		}

		// Fallback: no org context in the response. This happens when AuthKit
		// authenticated the user without scoping the session to an organization
		// — common for Google sign-in that doesn't go through an org-aware path.
		// Look up the user's memberships; if there's exactly one, switch the
		// session to that org via AuthenticateWithRefreshToken.
		if orgID == "" && resp.RefreshToken != "" {
			log.Printf("no org context — listing memberships for user %s", resp.User.ID)
			memResp, merr := usermanagement.ListOrganizationMemberships(ctx, usermanagement.ListOrganizationMembershipsOpts{
				UserID: resp.User.ID,
				Limit:  10,
			})
			if merr != nil {
				log.Printf("ListOrganizationMemberships failed: %v", merr)
			} else {
				log.Printf("found %d memberships for user", len(memResp.Data))
				var active *usermanagement.OrganizationMembership
				for i, m := range memResp.Data {
					log.Printf("  membership[%d] org=%s status=%s role=%s",
						i, m.OrganizationID, m.Status, m.Role.Slug)
					if m.Status == usermanagement.Active {
						active = &memResp.Data[i]
						break
					}
				}
				if active != nil {
					log.Printf("switching session to org %s (role=%s)", active.OrganizationID, active.Role.Slug)
					refreshResp, rerr := usermanagement.AuthenticateWithRefreshToken(ctx, usermanagement.AuthenticateWithRefreshTokenOpts{
						ClientID:       cfg.ClientID,
						RefreshToken:   resp.RefreshToken,
						OrganizationID: active.OrganizationID,
					})
					if rerr != nil {
						log.Printf("AuthenticateWithRefreshToken failed: %v", rerr)
						// Fall back to membership data alone.
						orgID = active.OrganizationID
						role = active.Role.Slug
					} else if claims, perr := auth.ParseAccessToken(refreshResp.AccessToken); perr == nil {
						log.Printf("after org-switch claims → org_id=%q role=%q", claims.OrganizationID, claims.Role)
						orgID = claims.OrganizationID
						role = claims.Role
					}
				}
			}
		}

		session := &auth.Session{
			UserID:         resp.User.ID,
			Email:          resp.User.Email,
			FirstName:      resp.User.FirstName,
			LastName:       resp.User.LastName,
			OrganizationID: orgID,
			Role:           role,
			ExpiresAt:      time.Now().Add(24 * time.Hour),
		}

		token, err := cfg.Seal(session)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.SetCookie(auth.CookieName, token, 24*60*60, "/", "localhost", false, true)
		c.Redirect(http.StatusFound, cfg.FrontendURL+"/")
	}
}

// Logout clears the session cookie. (For a full logout that also signs out of
// AuthKit, redirect to the WorkOS logout URL — out of scope for this prototype.)
func Logout(cfg *auth.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.SetCookie(auth.CookieName, "", -1, "/", "localhost", false, true)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
