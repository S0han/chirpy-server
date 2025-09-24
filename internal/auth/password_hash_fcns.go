package auth

import (
	"time"
	"fmt"
	
	"golang.org/x/crypto/bcrypt"
	
    "github.com/golang-jwt/jwt/v5"
    "github.com/google/uuid"
)

func HashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedPassword), nil
}

func CheckPasswordHash(password string, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer: 		"chirpy",
		IssuedAt: 	jwt.NewNumericDate(time.Now().UTC()),
		ExpiresAt: 	jwt.NewNumericDate(time.Now().UTC().Add(expiresIn)),
		Subject: 	userID.String(),
	})
	
	tokenString, err := token.SignedString([]byte(tokenSecret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
    claims := &jwt.RegisteredClaims{}
    token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
        if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
            return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
        }
        return []byte(tokenSecret), nil
    },
		jwt.WithLeeway(0),
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	)
    if err != nil {
        return uuid.Nil, err
    }
    if !token.Valid {
        return uuid.Nil, fmt.Errorf("invalid token")
    }

    userID, err := uuid.Parse(claims.Subject)
    if err != nil {
        return uuid.Nil, err
    }
    return userID, nil
}