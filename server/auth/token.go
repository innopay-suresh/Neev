package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type tokenHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// Claims are encoded into dashboard JWTs.
type Claims struct {
	Sub  string `json:"sub"`
	Role string `json:"role"`
	Iss  string `json:"iss"`
	Iat  int64  `json:"iat"`
	Exp  int64  `json:"exp"`
}

func base64URL(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeBase64URL(input string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(input)
}

// GenerateToken signs a JWT for the given user.
func GenerateToken(secret string, user *User, expiry time.Duration) (string, error) {
	if user == nil {
		return "", errors.New("user is nil")
	}
	headerJSON, _ := json.Marshal(tokenHeader{Alg: "HS256", Typ: "JWT"})
	now := time.Now().UTC()
	claimsJSON, err := json.Marshal(Claims{
		Sub:  user.Email,
		Role: user.Role,
		Iss:  "remote-agent",
		Iat:  now.Unix(),
		Exp:  now.Add(expiry).Unix(),
	})
	if err != nil {
		return "", err
	}
	unsigned := base64URL(headerJSON) + "." + base64URL(claimsJSON)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(unsigned))
	signature := mac.Sum(nil)
	return unsigned + "." + base64URL(signature), nil
}

// ParseToken verifies and decodes a JWT.
func ParseToken(secret, token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token")
	}
	unsigned := parts[0] + "." + parts[1]
	sig, err := decodeBase64URL(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(unsigned))
	expected := mac.Sum(nil)
	if !hmac.Equal(sig, expected) {
		return nil, errors.New("invalid signature")
	}
	payload, err := decodeBase64URL(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	now := time.Now().UTC().Unix()
	if claims.Exp > 0 && now > claims.Exp {
		return nil, errors.New("token expired")
	}
	if claims.Sub == "" || claims.Role == "" {
		return nil, errors.New("invalid token claims")
	}
	return &claims, nil
}

func roleRank(role string) int {
	switch strings.ToLower(role) {
	case RoleAdmin:
		return 3
	case RoleSupport:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

// RoleAllows reports whether actual role can access required role.
func RoleAllows(actual, required string) bool {
	return roleRank(actual) >= roleRank(required)
}
