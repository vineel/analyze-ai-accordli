package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const CookieName = "workos_session"

// Config holds the WorkOS prototype's runtime config.
type Config struct {
	ClientID       string
	APIKey         string
	CookiePassword string // must be at least 32 chars; first 32 used as AES-256 key
	RedirectURI    string
	FrontendURL    string
}

// Session is the per-user data we store in the encrypted cookie.
// Kept small on purpose — tokens stay server-side via the SDK calls.
type Session struct {
	UserID           string    `json:"user_id"`
	Email            string    `json:"email"`
	FirstName        string    `json:"first_name,omitempty"`
	LastName         string    `json:"last_name,omitempty"`
	OrganizationID   string    `json:"organization_id,omitempty"`
	OrganizationName string    `json:"organization_name,omitempty"`
	Role             string    `json:"role,omitempty"`
	ExpiresAt        time.Time `json:"expires_at"`
}

// AccessTokenClaims describes the JWT claims WorkOS issues on AuthKit access tokens.
// We only read the claims relevant to org/role; signature is not verified here
// (production code should verify against WorkOS's JWKS endpoint).
type AccessTokenClaims struct {
	OrganizationID string `json:"org_id"`
	Role           string `json:"role"`
	jwt.RegisteredClaims
}

// ParseAccessToken decodes the access token without verifying the signature.
func ParseAccessToken(token string) (*AccessTokenClaims, error) {
	claims := &AccessTokenClaims{}
	parser := jwt.NewParser()
	if _, _, err := parser.ParseUnverified(token, claims); err != nil {
		return nil, fmt.Errorf("parse access token: %w", err)
	}
	return claims, nil
}

func (cfg *Config) cookieKey() ([]byte, error) {
	if len(cfg.CookiePassword) < 32 {
		return nil, fmt.Errorf("WORKOS_COOKIE_PASSWORD must be >= 32 chars")
	}
	return []byte(cfg.CookiePassword[:32]), nil
}

// Seal encrypts the session into a base64-url string suitable for a cookie value.
func (cfg *Config) Seal(s *Session) (string, error) {
	plaintext, err := json.Marshal(s)
	if err != nil {
		return "", err
	}

	key, err := cfg.cookieKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.URLEncoding.EncodeToString(sealed), nil
}

// Unseal decrypts a cookie value back into a Session, returning an error if the
// cookie is invalid, tampered with, or the session has expired.
func (cfg *Config) Unseal(token string) (*Session, error) {
	sealed, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("decode session cookie: %w", err)
	}

	key, err := cfg.cookieKey()
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(sealed) < nonceSize {
		return nil, fmt.Errorf("session cookie too short")
	}

	nonce, ciphertext := sealed[:nonceSize], sealed[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt session cookie: %w", err)
	}

	var s Session
	if err := json.Unmarshal(plaintext, &s); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}

	if time.Now().After(s.ExpiresAt) {
		return nil, fmt.Errorf("session expired")
	}

	return &s, nil
}
