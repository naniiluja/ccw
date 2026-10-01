package store

import "fmt"

// OAuthCreds holds what one connection needs to refresh its access token. It is
// empty for a plain API-key connection.
type OAuthCreds struct {
	RefreshToken string
	TokenURL     string
	ClientID     string
	ClientSecret string
	ExpiresAt    string // RFC3339, empty means unknown
}

// oauthColumns are added to the connections table by migration 7. Each has an
// empty default so existing API-key rows are untouched.
var oauthColumns = []column{
	{"refresh_token", "TEXT NOT NULL DEFAULT ''"},
	{"token_url", "TEXT NOT NULL DEFAULT ''"},
	{"client_id", "TEXT NOT NULL DEFAULT ''"},
	{"client_secret", "TEXT NOT NULL DEFAULT ''"},
	{"expires_at", "TEXT NOT NULL DEFAULT ''"},
	// base_url overrides the provider's upstream for one connection: a
	// Cloudflare account, or a custom OpenAI-compatible provider.
	{"base_url", "TEXT NOT NULL DEFAULT ''"},
	// meta holds provider-specific values that are not secret, as a JSON
	// object: an Antigravity project id, a ChatGPT account id.
	{"meta", "TEXT NOT NULL DEFAULT '{}'"},
	// standby: an active connection that serves only when every other
	// active connection of its provider failed.
	{"standby", "INTEGER NOT NULL DEFAULT 0"},
}

// OAuth returns the refresh configuration for a connection.
func (s *Store) OAuth(id string) (OAuthCreds, error) {
	var c OAuthCreds
	err := s.DB.QueryRow(
		`SELECT refresh_token, token_url, client_id, client_secret, expires_at
		 FROM connections WHERE id = ?`, id).
		Scan(&c.RefreshToken, &c.TokenURL, &c.ClientID, &c.ClientSecret, &c.ExpiresAt)
	if err != nil {
		return OAuthCreds{}, fmt.Errorf("read oauth: %w", err)
	}
	return c, nil
}

// SetOAuth writes the refresh configuration for a connection.
func (s *Store) SetOAuth(id string, c OAuthCreds) error {
	_, err := s.DB.Exec(
		`UPDATE connections SET refresh_token=?, token_url=?, client_id=?,
		 client_secret=?, expires_at=? WHERE id=?`,
		c.RefreshToken, c.TokenURL, c.ClientID, c.ClientSecret, c.ExpiresAt, id)
	if err != nil {
		return fmt.Errorf("set oauth: %w", err)
	}
	return nil
}

// UpdateAccessToken replaces the stored access token and its expiry after a
// refresh. The refresh token and the rest of the OAuth config stay.
func (s *Store) UpdateAccessToken(id, accessToken, expiresAt string) error {
	_, err := s.DB.Exec(
		`UPDATE connections SET secret=?, expires_at=? WHERE id=?`,
		accessToken, expiresAt, id)
	if err != nil {
		return fmt.Errorf("update access token: %w", err)
	}
	return nil
}

// UpdateAfterRefresh stores a refreshed access token, its expiry, and a rotated
// refresh token. An empty newRefreshToken keeps the current one, because a
// provider that does not rotate returns none.
func (s *Store) UpdateAfterRefresh(id, accessToken, newRefreshToken, expiresAt string) error {
	if newRefreshToken == "" {
		return s.UpdateAccessToken(id, accessToken, expiresAt)
	}
	_, err := s.DB.Exec(
		`UPDATE connections SET secret=?, refresh_token=?, expires_at=? WHERE id=?`,
		accessToken, newRefreshToken, expiresAt, id)
	if err != nil {
		return fmt.Errorf("update after refresh: %w", err)
	}
	return nil
}
