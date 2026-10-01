package provider

import (
	"reflect"
	"testing"
)

// validAPIKeyDef is the smallest def Normalize accepts.
func validAPIKeyDef() Def {
	return Def{ID: "acme", BaseURL: "https://api.acme.dev/v1"}
}

func validCodeDef() Def {
	return Def{ID: "codeco", Kind: KindOAuthCode, BaseURL: "https://api.codeco.dev/v1",
		OAuth: &OAuth{AuthorizeURL: "https://auth.codeco.dev/authorize", TokenURL: "https://auth.codeco.dev/token",
			ClientID: "cid", RedirectURI: "http://localhost:1455/auth/callback"}}
}

func validDeviceDef() Def {
	return Def{ID: "devco", Kind: KindOAuthDevice, BaseURL: "https://api.devco.dev/v1",
		OAuth: &OAuth{DeviceCodeURL: "https://auth.devco.dev/device", TokenURL: "https://auth.devco.dev/token",
			ClientID: "cid"}}
}

// TestNormalizeFillsDefaultsAndTrims pins what Normalize writes back into a
// def it accepts.
func TestNormalizeFillsDefaultsAndTrims(t *testing.T) {
	d := Def{ID: "  ACME-1 ", BaseURL: " https://api.acme.dev/{accountId}/v1// ",
		Models: []string{" m-1 ", "", "  ", "m-2"}, OAuth: &OAuth{ClientID: "dropped"},
		Icon: "data:image/png;base64,AAAA", Color: "#A0b1C2",
		Headers: map[string]string{"anthropic-version": "2023-06-01"}}
	if err := d.Normalize(); err != nil {
		t.Fatal(err)
	}
	if d.ID != "acme-1" || d.Name != "acme-1" || d.Kind != KindAPIKey || d.API != "openai" {
		t.Errorf("defaults: id %q name %q kind %q api %q", d.ID, d.Name, d.Kind, d.API)
	}
	if d.BaseURL != "https://api.acme.dev/{accountId}/v1" {
		t.Errorf("baseUrl %q", d.BaseURL)
	}
	if !reflect.DeepEqual(d.Models, []string{"m-1", "m-2"}) {
		t.Errorf("models %q", d.Models)
	}
	if d.OAuth != nil {
		t.Errorf("an apikey def kept its oauth settings: %+v", d.OAuth)
	}
}

// TestNormalizeKeepsGivenNameKindAndAPI checks the defaults only fill blanks.
func TestNormalizeKeepsGivenNameKindAndAPI(t *testing.T) {
	for _, api := range []string{"openai", "anthropic", "responses", "typesafe"} {
		d := validAPIKeyDef()
		d.Name, d.API = "Acme", api
		if err := d.Normalize(); err != nil {
			t.Fatalf("api %q: %v", api, err)
		}
		if d.Name != "Acme" || d.API != api {
			t.Errorf("api %q: name %q api %q", api, d.Name, d.API)
		}
	}
	for _, mk := range []func() Def{validCodeDef, validDeviceDef} {
		d := mk()
		d.ModelsURL = "https://api.example.dev/models"
		if err := d.Normalize(); err != nil {
			t.Errorf("%s: %v", d.Kind, err)
		}
		if d.OAuth == nil {
			t.Errorf("%s: oauth settings were dropped", d.Kind)
		}
	}
}

// TestNormalizeErrors pins every refusal and the exact message, which names
// the field for the dashboard.
func TestNormalizeErrors(t *testing.T) {
	cases := []struct {
		name string
		mk   func() Def
		edit func(*Def)
		want string
	}{
		{"bad id", validAPIKeyDef, func(d *Def) { d.ID = "Acme_1" }, "id: lowercase letters, digits and -, up to 40"},
		{"empty id", validAPIKeyDef, func(d *Def) { d.ID = " " }, "id: lowercase letters, digits and -, up to 40"},
		{"builtin id", validAPIKeyDef, func(d *Def) { d.ID = "GitHub" }, `id: "github" is a built-in provider`},
		{"bad api", validAPIKeyDef, func(d *Def) { d.API = "gemini" }, "api: openai, anthropic, responses or typesafe"},
		{"empty baseUrl", validAPIKeyDef, func(d *Def) { d.BaseURL = "" }, "baseUrl: an http(s) URL"},
		{"ftp baseUrl", validAPIKeyDef, func(d *Def) { d.BaseURL = "ftp://x.dev" }, "baseUrl: an http(s) URL"},
		{"bad modelsUrl", validAPIKeyDef, func(d *Def) { d.ModelsURL = "javascript:x" }, "modelsUrl: an http(s) URL, or none"},
		{"http icon", validAPIKeyDef, func(d *Def) { d.Icon = "http://x.dev/i.png" }, "icon: an https: or data:image/ URL"},
		{"data html icon", validAPIKeyDef, func(d *Def) { d.Icon = "data:text/html,x" }, "icon: an https: or data:image/ URL"},
		{"javascript icon", validAPIKeyDef, func(d *Def) { d.Icon = "javascript:alert(1)" }, "icon: an https: or data:image/ URL"},
		{"bad color", validAPIKeyDef, func(d *Def) { d.Color = "red" }, "color: #rrggbb"},
		{"short color", validAPIKeyDef, func(d *Def) { d.Color = "#abc" }, "color: #rrggbb"},
		{"blank header", validAPIKeyDef, func(d *Def) { d.Headers = map[string]string{" ": "v"} }, `headers: " " is not a header name`},
		{"colon header", validAPIKeyDef, func(d *Def) { d.Headers = map[string]string{"a:b": "v"} }, `headers: "a:b" is not a header name`},
		{"none without models", validAPIKeyDef, func(d *Def) { d.ModelsURL, d.Models = "none", []string{" "} }, "models: list the models when the provider has no model list"},
		{"bad kind", validAPIKeyDef, func(d *Def) { d.Kind = "password" }, "kind: apikey, oauth-code or oauth-device"},
		{"oauth missing", validCodeDef, func(d *Def) { d.OAuth = nil }, "oauth: the sign-in settings are required"},
		{"client id missing", validDeviceDef, func(d *Def) { d.OAuth.ClientID = "" }, "oauth.clientId is required"},
		{"token url bad", validCodeDef, func(d *Def) { d.OAuth.TokenURL = "/token" }, "oauth.tokenUrl: an http(s) URL"},
		{"verify url javascript", validDeviceDef, func(d *Def) { d.OAuth.VerifyURL = "javascript:alert(1)" }, "oauth.verifyUrl: an http(s) URL, or none"},
		{"authorize url bad", validCodeDef, func(d *Def) { d.OAuth.AuthorizeURL = "" }, "oauth.authorizeUrl: an http(s) URL"},
		{"redirect missing", validCodeDef, func(d *Def) { d.OAuth.RedirectURI = "" }, "oauth.redirectUri is required (the one the provider's app registered)"},
		{"device url bad", validDeviceDef, func(d *Def) { d.OAuth.DeviceCodeURL = "data:x" }, "oauth.deviceCodeUrl: an http(s) URL"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := c.mk()
			c.edit(&d)
			err := d.Normalize()
			if err == nil {
				t.Fatalf("accepted, want %q", c.want)
			}
			if err.Error() != c.want {
				t.Errorf("error %q, want %q", err, c.want)
			}
		})
	}
}

// TestNormalizeStopsAtTheFirstError pins the check order: a bad id leaves the
// later defaults unfilled.
func TestNormalizeStopsAtTheFirstError(t *testing.T) {
	d := Def{ID: "Bad_ID", BaseURL: "nope", Kind: "nope"}
	if err := d.Normalize(); err == nil || err.Error() != "id: lowercase letters, digits and -, up to 40" {
		t.Fatalf("error %v", err)
	}
	if d.Name != "" || d.API != "" || d.ID != "bad_id" {
		t.Errorf("after a bad id: name %q api %q id %q", d.Name, d.API, d.ID)
	}
}
