package http

import (
	"encoding/base64"
	"log"
	"net/http"
	"os"
	"server/data/database"
	"server/proxy/user"
	"strings"
)

func Authenticate(req *http.Request) (bool, map[string]string) {
	authHeader := req.Header.Get("Proxy-Authorization")
	if authHeader == "" {
		return false, nil
	}

	if !strings.HasPrefix(authHeader, "Basic ") {
		log.Println("Unsupported authentication method:", authHeader)
		return false, nil
	}

	encoded := strings.TrimPrefix(authHeader, "Basic ")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		log.Println("Failed to decode credentials:", err)
		return false, nil
	}

	credentials := string(decoded)
	parts := strings.SplitN(credentials, ":", 2)
	if len(parts) != 2 {
		log.Println("Invalid credentials format")
		return false, nil
	}

	username, password := parts[0], parts[1]

	credits, err := database.GetCredits(password)
	_ = credits
	// TODO: create local user struct to consume credits

	if err != nil && os.Getenv("DEBUG_MODE") != "1" {
		log.Println("Authentication failed")
		return false, nil
	}

	return true, user.ParseParams(username)
}
