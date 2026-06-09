package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/adham/hotel-qr-ordering/internal/auth"
)

// JWTAuthMiddleware validates the JWT token found in the Authorization header or a cookie
func JWTAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// First check cookie
		tokenString, err := c.Cookie("admin_token")
		if err != nil || tokenString == "" {
			// Fallback to Authorization header
			authHeader := c.GetHeader("Authorization")
			if authHeader == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized, missing token"})
				return
			}
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized, invalid auth format"})
				return
			}
			tokenString = parts[1]
		}

		claims, err := auth.ValidateToken(tokenString)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized, invalid token"})
			return
		}

		// Set claims in context
		c.Set("user_id", claims.UserID)
		c.Set("property_id", claims.PropertyID)
		
		c.Next()
	}
}
