package control

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type sellerClaims struct {
	SellerID string `json:"sid"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Kind     string `json:"kind"`
	jwt.RegisteredClaims
}

// IssueSellerToken returns an HS256 JWT for an authenticated seller.
func IssueSellerToken(secret []byte, sellerID, email, name string) (string, error) {
	if len(secret) == 0 {
		return "", errors.New("empty jwt secret")
	}
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, sellerClaims{
		SellerID: sellerID,
		Name:     name,
		Email:    email,
		Kind:     "seller",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sellerID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour * 30)),
		},
	})
	return tok.SignedString(secret)
}

// ParseSellerToken validates a seller JWT.
func ParseSellerToken(secret []byte, token string) (sellerID, email, name string, err error) {
	if len(secret) == 0 || token == "" {
		return "", "", "", errors.New("invalid token")
	}
	var c sellerClaims
	_, err = jwt.ParseWithClaims(token, &c, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return secret, nil
	})
	if err != nil {
		return "", "", "", err
	}
	if c.Kind != "seller" {
		return "", "", "", errors.New("not a seller token")
	}
	if c.SellerID == "" {
		c.SellerID = c.Subject
	}
	return c.SellerID, c.Email, c.Name, nil
}
