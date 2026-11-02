// Package auth verifies HS256 JWTs. In production the gateway/IdP (Cognito,
// Okta, Keycloak) issues RS256/ES256 tokens verified via JWKS; this keeps the
// same claim contract (tenant + scopes) with zero dependencies.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Claims struct {
	Subject string   `json:"sub"`
	Tenant  string   `json:"tenant"`
	Scopes  []string `json:"scopes"`
	Exp     int64    `json:"exp"`
	Nbf     int64    `json:"nbf,omitempty"`
}

var ErrInvalidToken = errors.New("invalid token")

func (c Claims) HasScope(s string) bool {
	for _, x := range c.Scopes {
		if x == s {
			return true
		}
	}
	return false
}

var enc = base64.RawURLEncoding

func Sign(secret []byte, c Claims) string {
	h := enc.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	p, _ := json.Marshal(c)
	body := h + "." + enc.EncodeToString(p)
	return body + "." + enc.EncodeToString(mac(secret, body))
}

func Verify(secret []byte, token string, now time.Time) (Claims, error) {
	var c Claims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return c, ErrInvalidToken
	}
	hdrRaw, err := enc.DecodeString(parts[0])
	if err != nil {
		return c, ErrInvalidToken
	}
	var hdr struct {
		Alg string `json:"alg"`
	}
	// Pin the algorithm: rejects alg=none / algorithm-confusion attacks.
	if json.Unmarshal(hdrRaw, &hdr) != nil || hdr.Alg != "HS256" {
		return c, ErrInvalidToken
	}
	sig, err := enc.DecodeString(parts[2])
	if err != nil || !hmac.Equal(sig, mac(secret, parts[0]+"."+parts[1])) { // constant-time
		return c, ErrInvalidToken
	}
	payload, err := enc.DecodeString(parts[1])
	if err != nil || json.Unmarshal(payload, &c) != nil {
		return c, ErrInvalidToken
	}
	if c.Exp == 0 || now.Unix() >= c.Exp || (c.Nbf != 0 && now.Unix() < c.Nbf) || c.Tenant == "" {
		return Claims{}, ErrInvalidToken
	}
	return c, nil
}

func mac(secret []byte, s string) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(s))
	return m.Sum(nil)
}
