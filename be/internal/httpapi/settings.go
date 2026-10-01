package httpapi

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/naniiluja/ccw/internal/websearch"
)

// settingsWebsearch is the web search part of GET /api/settings. The key is
// reported only as a flag.
type settingsWebsearch struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Count    int    `json:"count"`
	URL      string `json:"url"`
	KeySet   bool   `json:"keySet"`
}

// settingsView is the body of GET /api/settings: the effective configuration
// that only environment variables can change. It never carries a secret.
type settingsView struct {
	AuthMode          string            `json:"authMode"`
	SessionTTLSeconds int               `json:"sessionTtlSeconds"`
	Timezone          string            `json:"timezone"`
	Websearch         settingsWebsearch `json:"websearch"`
}

// getSettings serves the effective settings, read-only.
func (a *api) getSettings(w http.ResponseWriter, r *http.Request) {
	v := settingsView{
		AuthMode:  "none",
		Timezone:  reportLocation().String(),
		Websearch: settingsWebsearch{Model: searchModel(), Count: websearch.DefaultCount},
	}
	if a.auth != nil {
		v.AuthMode = "password"
		v.SessionTTLSeconds = a.auth.TTLSeconds()
	}
	if c := websearch.FromEnv(); c != nil {
		v.Websearch.Provider = c.Provider
		v.Websearch.KeySet = c.Key != ""
		v.Websearch.URL = publicURL(c.BaseURL)
		v.Websearch.Count = c.EffectiveCount()
	}
	writeJSON(w, v)
}

// publicURL drops what could carry a credential from a URL: the user info, the
// query and the fragment. A URL that does not parse is reported as empty.
func publicURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	u.User, u.RawQuery, u.Fragment = nil, "", ""
	return strings.TrimRight(u.String(), "/")
}
