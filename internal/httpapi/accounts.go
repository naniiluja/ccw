package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/naniiluja/ccw/internal/provider"
)

// accounts lists the connections so a caller can choose an id.
// The listing type carries no credential, which is what keeps this endpoint safe.
func (a *api) accounts(w http.ResponseWriter, r *http.Request) {
	list, err := a.store.ListConnections()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot read connections")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"accounts": list})
}

// customID is the shape of a custom provider id, which prefixes model names.
var customID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// createAccount stores a new connection from the dashboard form, then returns
// to the dashboard. The secret is taken by value and never echoed back.
//
// A registered provider may need an account id (Cloudflare), no key at all, or
// an OAuth sign-in. Any other provider id is a custom OpenAI-compatible
// provider and must come with its base URL.
func (a *api) createAccount(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "bad form")
		return
	}
	prov := strings.TrimSpace(r.PostFormValue("provider"))
	// A pasted key often carries a newline or a stray space, which no
	// provider accepts in a header.
	secret := strings.TrimSpace(r.PostFormValue("secret"))
	baseURL := strings.TrimRight(strings.TrimSpace(r.PostFormValue("base_url")), "/")
	p, registered := provider.Lookup(prov)
	accountID := ""
	switch {
	case prov == "":
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	case registered && p.Setup == "account":
		acct := strings.TrimSpace(r.PostFormValue("account_id"))
		if acct == "" || secret == "" {
			writeError(w, http.StatusBadRequest, "account id and key are required")
			return
		}
		baseURL = p.WithAccount(acct)
		accountID = acct
	case registered && p.Setup == "oauth":
		writeError(w, http.StatusBadRequest, "this provider's accounts sign in with OAuth; they cannot be added with a key")
		return
	case registered && p.Setup == "none":
		secret = p.DefaultSecret
	case registered:
		if secret == "" {
			writeError(w, http.StatusBadRequest, "key is required")
			return
		}
	default:
		if !customID.MatchString(prov) {
			writeError(w, http.StatusBadRequest, "provider id must be lowercase letters, digits or -")
			return
		}
		if u, err := url.Parse(baseURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			writeError(w, http.StatusBadRequest, "a custom provider needs an http(s) base URL")
			return
		}
		// A new custom endpoint becomes a declared provider, so its settings
		// live in one place and every account of it follows them.
		d := provider.Def{ID: prov, Kind: provider.KindAPIKey, API: r.PostFormValue("api"), BaseURL: baseURL}
		if err := d.Normalize(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := a.store.PutProviderDef(d); err != nil {
			writeError(w, http.StatusInternalServerError, "cannot store the provider")
			return
		}
		a.loadDefs()
		baseURL = ""
	}
	c, err := a.store.CreateConnection(prov, r.PostFormValue("label"), secret)
	if err == nil && baseURL != "" {
		err = a.store.SetBaseURL(c.ID, baseURL)
	}
	if err == nil && accountID != "" {
		err = a.store.SetMeta(c.ID, map[string]string{"accountId": accountID})
	}
	if err != nil {
		// A base URL or account id that did not store leaves a connection that
		// routes to a broken target. Roll the row back rather than keep it.
		if c.ID != "" {
			a.store.DeleteConnection(c.ID)
		}
		writeError(w, http.StatusInternalServerError, "cannot create connection")
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

// deleteAccount removes one connection, then returns to the dashboard.
func (a *api) deleteAccount(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteConnection(r.PathValue("id")); err != nil {
		writeError(w, http.StatusNotFound, "unknown connection")
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

// usage serves the daily token counters, newest day first.
func (a *api) usage(w http.ResponseWriter, r *http.Request) {
	rows, err := a.store.Usage()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot read usage")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"usage": rows})
}

// modelsForAccount fetches the provider's model list for one connection, from
// the server side so the dashboard (which holds a session, not the bearer token)
// can show it. It returns the provider's own JSON unchanged.
func (a *api) modelsForAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	conn, err := a.connection(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "unknown connection")
		return
	}
	if _, ok := a.providerFor(conn); !ok {
		writeError(w, http.StatusNotFound, "provider not supported in this build")
		return
	}
	resp, err := a.getModels(r.Context(), conn)
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream unreachable")
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// loadDefs publishes the declared providers to the registry.
func (a *api) loadDefs() {
	defs, err := a.store.ProviderDefs()
	if err != nil {
		log.Printf("provider defs: %v", err)
		return
	}
	provider.SetDeclared(defs)
}

// migrateCustomEndpoints turns the custom endpoints of older versions (a base
// URL kept on each connection) into declared providers, once.
func (a *api) migrateCustomEndpoints() {
	conns, err := a.store.ListConnections()
	if err != nil {
		return
	}
	for _, c := range conns {
		if c.BaseURL == "" || provider.Builtin(c.Provider) {
			continue
		}
		if _, ok := provider.Declared(c.Provider); ok {
			continue
		}
		d := provider.Def{ID: c.Provider, Kind: provider.KindAPIKey, API: c.Meta["api"], BaseURL: c.BaseURL}
		if err := d.Normalize(); err != nil {
			log.Printf("provider defs: cannot migrate %s: %v", c.Provider, err)
			continue
		}
		if err := a.store.PutProviderDef(d); err != nil {
			log.Printf("provider defs: %v", err)
			continue
		}
		a.loadDefs()
		log.Printf("provider defs: %s is now a declared provider", c.Provider)
	}
}

// maskedValue stands in for a credential that a caller may not read.
const maskedValue = "••••"

// maskDef hides the credentials a declared provider carries: the OAuth client
// secret, and the static header values, which hold an organisation key. It
// copies what it changes, so the stored def and the registry keep their value.
func maskDef(d provider.Def) provider.Def {
	if d.OAuth != nil && d.OAuth.ClientSecret != "" {
		o := *d.OAuth
		o.ClientSecret = maskedValue
		d.OAuth = &o
	}
	if len(d.Headers) > 0 {
		h := make(map[string]string, len(d.Headers))
		for k := range d.Headers {
			h[k] = maskedValue
		}
		d.Headers = h
	}
	return d
}

// listDefs serves the declared providers.
func (a *api) listDefs(w http.ResponseWriter, r *http.Request) {
	defs, err := a.store.ProviderDefs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot read providers")
		return
	}
	if !principalOf(r).admin {
		for i := range defs {
			defs[i] = maskDef(defs[i])
		}
	}
	writeJSON(w, map[string]any{"providers": defs})
}

// getDef serves one declared provider.
func (a *api) getDef(w http.ResponseWriter, r *http.Request) {
	d, ok := provider.Declared(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "no declared provider of that id")
		return
	}
	if !principalOf(r).admin {
		d = maskDef(d)
	}
	writeJSON(w, d)
}

// putDef declares a provider or replaces its declaration: a Def as JSON.
func (a *api) putDef(w http.ResponseWriter, r *http.Request) {
	var d provider.Def
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&d); err != nil {
		writeError(w, http.StatusBadRequest, "bad json: "+err.Error())
		return
	}
	saved, err := a.saveDef(d)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, saved)
}

func (a *api) saveDef(d provider.Def) (provider.Def, error) {
	d.Builtin = false
	if err := d.Normalize(); err != nil {
		return d, err
	}
	if err := a.store.PutProviderDef(d); err != nil {
		return d, err
	}
	a.loadDefs()
	// Its model list may live elsewhere now.
	a.cat.mu.Lock()
	delete(a.cat.m, d.ID)
	a.cat.mu.Unlock()
	return d, nil
}

// deleteDef removes a declaration; a provider with accounts is kept.
func (a *api) deleteDef(w http.ResponseWriter, r *http.Request) {
	if err := a.removeDef(r.PathValue("id")); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"deleted": true})
}

func (a *api) removeDef(id string) error {
	conns, _ := a.store.ListConnections()
	n := 0
	for _, c := range conns {
		if c.Provider == id {
			n++
		}
	}
	if n > 0 {
		return fmt.Errorf("delete its %d account(s) first", n)
	}
	if err := a.store.DeleteProviderDef(id); err != nil {
		return fmt.Errorf("no declared provider of that id")
	}
	a.loadDefs()
	return nil
}

// setActive turns a connection on or off from a JSON body
// {"active":bool,"standby":bool}; standby left out keeps the current mark.
func (a *api) setActive(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Active  bool  `json:"active"`
		Standby *bool `json:"standby"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad json")
		return
	}
	id := r.PathValue("id")
	if err := a.store.SetActive(id, body.Active); err != nil {
		writeError(w, http.StatusNotFound, "unknown connection")
		return
	}
	// Off means never called, so it drops a standby mark: switched on again
	// without one, the account takes turns like any other.
	if !body.Active || body.Standby != nil {
		standby := body.Active && body.Standby != nil && *body.Standby
		if err := a.store.SetStandby(id, standby); err != nil {
			writeError(w, http.StatusInternalServerError, "cannot update connection")
			return
		}
	}
	writeJSON(w, map[string]any{"ok": true, "active": body.Active})
}

// providers lists the providers this build can proxy and what a new
// connection of each needs, so the dashboard can offer them and mark a stored
// connection it cannot reach.
func (a *api) providers(w http.ResponseWriter, r *http.Request) {
	type info struct {
		ID    string `json:"id"`
		Setup string `json:"setup"`
		// Auth is "oauth" for an account that signs in as a real tool, or
		// "apikey" for a documented API reached with a key.
		Auth string `json:"auth"`
		// A declared provider: its name, colour, icon URL, sign-in flow.
		Declared bool   `json:"declared,omitempty"`
		Name     string `json:"name,omitempty"`
		Color    string `json:"color,omitempty"`
		IconURL  string `json:"iconUrl,omitempty"`
		Flow     string `json:"flow,omitempty"`
		API      string `json:"api,omitempty"`
	}
	out := []info{}
	for _, id := range provider.IDs() {
		p, _ := provider.Lookup(id)
		setup := p.Setup
		if setup == "" {
			setup = "key"
		}
		auth := "apikey"
		if p.Setup == "oauth" || p.Exchange != "" {
			auth = "oauth"
		}
		in := info{ID: id, Setup: setup, Auth: auth}
		if d, ok := provider.Declared(id); ok {
			in.Declared, in.Name, in.Color, in.IconURL, in.API = true, d.Name, d.Color, d.Icon, d.API
			switch d.Kind {
			case provider.KindOAuthCode:
				in.Flow = "code"
			case provider.KindOAuthDevice:
				in.Flow = "device"
			}
		}
		out = append(out, in)
	}
	writeJSON(w, map[string]any{"providers": out})
}

// providerModelList serves a provider's models from the same catalog as
// /v1/models (cached, filtered, with the provider's fallback list).
func (a *api) providerModelList(w http.ResponseWriter, r *http.Request) {
	prov := r.PathValue("id")
	ids := a.providerModels(r.Context(), prov)
	a.cat.mu.Lock()
	ok := a.cat.m[prov].ok
	a.cat.mu.Unlock()
	if ids == nil {
		ids = []string{}
	}
	writeJSON(w, map[string]any{"models": ids, "ok": ok})
}

// setLabel renames a connection from {"label":"…"}.
func (a *api) setLabel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil || strings.TrimSpace(body.Label) == "" {
		writeError(w, http.StatusBadRequest, "a label is required")
		return
	}
	if err := a.store.SetLabel(r.PathValue("id"), strings.TrimSpace(body.Label)); err != nil {
		writeError(w, http.StatusNotFound, "unknown connection")
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// uiSettingKeys are the dashboard preferences kept on the server, so every
// browser shows the same view.
var uiSettingKeys = map[string]bool{"quota-view": true}

// getUISetting serves one dashboard preference as stored (JSON).
func (a *api) getUISetting(w http.ResponseWriter, r *http.Request) {
	k := r.PathValue("key")
	if !uiSettingKeys[k] {
		writeError(w, http.StatusNotFound, "unknown setting")
		return
	}
	v, err := a.store.GetSetting("ui." + k)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot read setting")
		return
	}
	if v == "" {
		v = "{}"
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(v))
}

// setUISetting stores one dashboard preference; the body must be JSON.
func (a *api) setUISetting(w http.ResponseWriter, r *http.Request) {
	k := r.PathValue("key")
	if !uiSettingKeys[k] {
		writeError(w, http.StatusNotFound, "unknown setting")
		return
	}
	var v json.RawMessage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&v); err != nil {
		writeError(w, http.StatusBadRequest, "bad json")
		return
	}
	if err := a.store.SetSetting("ui."+k, string(v)); err != nil {
		writeError(w, http.StatusInternalServerError, "cannot save setting")
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
