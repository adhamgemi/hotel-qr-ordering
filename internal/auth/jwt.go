package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var JWTSecret = []byte("super_secret_jwt_key_for_hotel_saas_dev") // In production, load from ENV

type Claims struct {
	UserID     string `json:"user_id"`
	PropertyID string `json:"property_id"`
	jwt.RegisteredClaims
}

// GenerateToken generates a JWT for an admin user
func GenerateToken(userID, propertyID string) (string, error) {
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &Claims{
		UserID:     userID,
		PropertyID: propertyID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(JWTSecret)
}

// ValidateToken parses and validates a JWT token string
func ValidateToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return JWTSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}
