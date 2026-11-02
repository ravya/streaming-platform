// tokengen mints a dev JWT:  JWT_SECRET=... go run ./cmd/tokengen flipkart
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/ravya/retail-insights-platform/internal/auth"
)

func main() {
	if len(os.Args) < 2 || os.Getenv("JWT_SECRET") == "" {
		fmt.Fprintln(os.Stderr, "usage: JWT_SECRET=... tokengen <tenant>")
		os.Exit(2)
	}
	fmt.Println(auth.Sign([]byte(os.Getenv("JWT_SECRET")), auth.Claims{
		Subject: "dev-user", Tenant: os.Args[1], Scopes: []string{"insights:read"}, Exp: time.Now().Add(time.Hour).Unix(),
	}))
}
