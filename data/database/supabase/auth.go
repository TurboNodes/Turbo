package supabase

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const authHTTPTimeout = 12 * time.Second

// SessionTokens is the Supabase session payload sent to paired nodes.
type SessionTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// IssueUserSessionTokens mints a fresh Supabase session for the node owner.
func IssueUserSessionTokens(userID string) (SessionTokens, error) {
	if userID == "" {
		return SessionTokens{}, errors.New("empty user id")
	}

	baseURL := strings.TrimRight(os.Getenv("SUPABASE_URL"), "/")
	secretKey := os.Getenv("SUPABASE_SECRET_KEY")
	if baseURL == "" || secretKey == "" {
		return SessionTokens{}, errors.New("SUPABASE_URL and SUPABASE_SECRET_KEY must be set")
	}

	email, err := getUserEmail(userID)
	if err != nil {
		return SessionTokens{}, err
	}

	hashedToken, err := adminGenerateMagicLink(baseURL, secretKey, email)
	if err != nil {
		return SessionTokens{}, err
	}

	return verifyMagicLink(baseURL, secretKey, hashedToken)
}

func getUserEmail(userID string) (string, error) {
	if SupabaseDB == nil {
		return "", errors.New("database not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbOpTimeout)
	defer cancel()

	var email sql.NullString
	err := SupabaseDB.QueryRowContext(
		ctx,
		`SELECT email FROM auth.users WHERE id = $1`,
		userID,
	).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("user %s not found", userID)
	}
	if err != nil {
		return "", err
	}
	if !email.Valid || email.String == "" {
		return "", fmt.Errorf("user %s has no email", userID)
	}
	return email.String, nil
}

func adminGenerateMagicLink(baseURL, secretKey, email string) (string, error) {
	body, err := json.Marshal(map[string]string{
		"type":  "magiclink",
		"email": email,
	})
	if err != nil {
		return "", err
	}

	var resp struct {
		HashedToken string `json:"hashed_token"`
		Error       string `json:"error"`
		Msg         string `json:"msg"`
	}
	if err := authRequest(baseURL+"/auth/v1/admin/generate_link", secretKey, body, &resp); err != nil {
		return "", err
	}
	if resp.HashedToken == "" {
		if resp.Msg != "" {
			return "", errors.New(resp.Msg)
		}
		if resp.Error != "" {
			return "", errors.New(resp.Error)
		}
		return "", errors.New("generate_link returned no token")
	}
	return resp.HashedToken, nil
}

func verifyMagicLink(baseURL, secretKey, hashedToken string) (SessionTokens, error) {
	body, err := json.Marshal(map[string]string{
		"type":       "magiclink",
		"token_hash": hashedToken,
	})
	if err != nil {
		return SessionTokens{}, err
	}

	var resp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
		Msg          string `json:"msg"`
	}
	if err := authRequest(baseURL+"/auth/v1/verify", secretKey, body, &resp); err != nil {
		return SessionTokens{}, err
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		if resp.Msg != "" {
			return SessionTokens{}, errors.New(resp.Msg)
		}
		if resp.Error != "" {
			return SessionTokens{}, errors.New(resp.Error)
		}
		return SessionTokens{}, errors.New("verify returned no session")
	}
	return SessionTokens{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresIn:    resp.ExpiresIn,
	}, nil
}

func authRequest(url, secretKey string, body []byte, out any) error {
	ctx, cancel := context.WithTimeout(context.Background(), authHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	// New secret keys (sb_secret_...) are not JWTs — send only on apikey.
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", secretKey)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("auth request failed (%d): %s", res.StatusCode, strings.TrimSpace(string(raw)))
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode auth response: %w", err)
	}
	return nil
}
