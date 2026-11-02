package auth

import (
	"strings"
	"testing"
	"time"
)

func TestVerify(t *testing.T) {
	sec := []byte("s3cret")
	now := time.Now()
	good := Sign(sec, Claims{Tenant: "a", Scopes: []string{"x"}, Exp: now.Add(time.Hour).Unix()})
	expired := Sign(sec, Claims{Tenant: "a", Exp: now.Add(-time.Hour).Unix()})
	noTenant := Sign(sec, Claims{Exp: now.Add(time.Hour).Unix()})
	parts := strings.Split(good, ".")
	tampered := parts[0] + "." + parts[1] + "x." + parts[2]
	algNone := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0." + parts[1] + "."

	cases := map[string]struct {
		tok string
		ok  bool
	}{
		"valid": {good, true}, "expired": {expired, false}, "no tenant": {noTenant, false},
		"tampered": {tampered, false}, "alg none": {algNone, false}, "garbage": {"abc", false},
		"wrong secret": {Sign([]byte("other"), Claims{Tenant: "a", Exp: now.Add(time.Hour).Unix()}), false},
	}
	for name, c := range cases {
		_, err := Verify(sec, c.tok, now)
		if (err == nil) != c.ok {
			t.Errorf("%s: got err=%v, want ok=%v", name, err, c.ok)
		}
	}
}
