package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	// ContextKeyAccountID is the gin context key for the resolved account ID.
	ContextKeyAccountID = "account_id"
	// ContextKeyClaims is the gin context key for the parsed JWT claims.
	ContextKeyClaims = "jwt_claims"
)

// AccountLookup resolves a Zitadel subject (sub) to a lurus-identity account ID.
// The implementation typically checks Redis first, then falls back to the DB.
type AccountLookup func(ctx context.Context, zitadelSub string) (int64, error)

// JWTMiddleware is a Gin middleware factory for JWT validation.
type JWTMiddleware struct {
	validator *Validator
	lookup    AccountLookup
}

// NewJWTMiddleware creates the middleware. lookup is called after signature
// validation to resolve the Zitadel sub to the internal account ID.
func NewJWTMiddleware(v *Validator, lookup AccountLookup) *JWTMiddleware {
	return &JWTMiddleware{validator: v, lookup: lookup}
}

// Auth returns a Gin HandlerFunc that validates the Bearer JWT and sets
// account_id + jwt_claims in the context. Aborts with 401 on any failure.
func (m *JWTMiddleware) Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := extractBearerToken(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		claims, err := m.validator.Validate(c.Request.Context(), token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		accountID, err := m.lookup(c.Request.Context(), claims.Sub)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "account not found"})
			return
		}

		c.Set(ContextKeyAccountID, accountID)
		c.Set(ContextKeyClaims, claims)
		c.Next()
	}
}

// AdminAuth returns a Gin HandlerFunc that validates the JWT AND requires an
// admin role. Returns 401 for missing/invalid tokens, 403 for missing role.
func (m *JWTMiddleware) AdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := extractBearerToken(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		claims, err := m.validator.Validate(c.Request.Context(), token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		if !m.validator.HasAdminRole(claims) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin role required"})
			return
		}

		accountID, err := m.lookup(c.Request.Context(), claims.Sub)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "account not found"})
			return
		}

		c.Set(ContextKeyAccountID, accountID)
		c.Set(ContextKeyClaims, claims)
		c.Next()
	}
}

// extractBearerToken extracts the token from Authorization: Bearer <token>.
func extractBearerToken(c *gin.Context) (string, error) {
	header := c.GetHeader("Authorization")
	if header == "" {
		return "", &errUnauthorized{"missing Authorization header"}
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", &errUnauthorized{"Authorization header must use Bearer scheme"}
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return "", &errUnauthorized{"empty bearer token"}
	}
	return token, nil
}

type errUnauthorized struct{ msg string }

func (e *errUnauthorized) Error() string { return e.msg }

// GetAccountID retrieves the account ID set by Auth middleware.
// Returns 0 if not set (should not happen on authenticated routes).
func GetAccountID(c *gin.Context) int64 {
	v, _ := c.Get(ContextKeyAccountID)
	id, _ := v.(int64)
	return id
}

// GetClaims retrieves the JWT claims set by Auth middleware.
func GetClaims(c *gin.Context) *Claims {
	v, _ := c.Get(ContextKeyClaims)
	claims, _ := v.(*Claims)
	return claims
}
