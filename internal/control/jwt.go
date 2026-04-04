package control

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type buyerClaims struct {
	BuyerID string `json:"bid"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Kind    string `json:"kind"`
	jwt.RegisteredClaims
}

// IssueBuyerToken returns an HS256 JWT for an authenticated buyer.
func IssueBuyerToken(secret []byte, buyerID, email, name string) (string, error) {
	if len(secret) == 0 {
		return "", errors.New("empty jwt secret")
	}
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, buyerClaims{
		BuyerID: buyerID,
		Name:    name,
		Email:   email,
		Kind:    "buyer",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   buyerID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour * 30)),
		},
	})
	return tok.SignedString(secret)
}

// ParseBuyerToken validates the token and returns buyer id, email, and display name.
func ParseBuyerToken(secret []byte, token string) (buyerID, email, name string, err error) {
	if len(secret) == 0 || token == "" {
		return "", "", "", errors.New("invalid token")
	}
	var c buyerClaims
	_, err = jwt.ParseWithClaims(token, &c, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return secret, nil
	})
	if err != nil {
		return "", "", "", err
	}
	if c.Kind != "" && c.Kind != "buyer" {
		return "", "", "", errors.New("not a buyer token")
	}
	if c.BuyerID == "" {
		c.BuyerID = c.Subject
	}
	return c.BuyerID, c.Email, c.Name, nil
}
