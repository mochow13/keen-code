package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewOAuthManagerSetsDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	manager := NewOAuthManager(nil)
	if manager.Store == nil || manager.HTTPClient == nil || manager.OpenBrowser == nil {
		t.Fatal("NewOAuthManager did not set defaults")
	}
}

func TestGenerateState(t *testing.T) {
	state, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() error = %v", err)
	}
	if len(state) < 40 {
		t.Fatalf("GenerateState() returned short state %q", state)
	}
}

func TestAuthorizationURL(t *testing.T) {
	manager := NewOAuthManager(NewStoreAt(filepath.Join(t.TempDir(), "auth.json")))
	manager.Issuer = "https://issuer.example/"
	manager.ClientID = "client"
	manager.RedirectURI = "http://localhost/callback"

	authURL, err := url.Parse(manager.AuthorizationURL("challenge", "state"))
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	if authURL.Path != "/oauth/authorize" || authURL.Query().Get("code_challenge") != "challenge" || authURL.Query().Get("state") != "state" {
		t.Fatalf("unexpected authorization URL %s", authURL)
	}
}

func TestHasCredentialAndValidAccessToken(t *testing.T) {
	store := NewStoreAt(filepath.Join(t.TempDir(), "auth.json"))
	manager := NewOAuthManager(store)
	if manager.HasCredential("provider") {
		t.Fatal("HasCredential returned true for an empty store")
	}

	cred := OAuthCredential{AccessToken: "access", RefreshToken: "refresh", ExpiresAt: time.Now().Add(time.Hour)}
	if err := store.Set("provider", cred); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if !manager.HasCredential("provider") {
		t.Fatal("HasCredential returned false for a stored credential")
	}
	got, err := manager.ValidAccessToken(context.Background(), "provider")
	if err != nil || got.AccessToken != "access" {
		t.Fatalf("ValidAccessToken() = %#v, %v", got, err)
	}
	if _, err := manager.ValidAccessToken(context.Background(), "missing"); err == nil {
		t.Fatal("ValidAccessToken succeeded without a credential")
	}
}

func TestPostTokenRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "HTTP error", status: http.StatusUnauthorized, body: "denied", want: "HTTP 401"},
		{name: "missing access token", status: http.StatusOK, body: `{}`, want: "missing access_token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer server.Close()

			manager := NewOAuthManager(NewStoreAt(filepath.Join(t.TempDir(), "auth.json")))
			manager.Issuer = server.URL
			_, err := manager.postToken(context.Background(), url.Values{"grant_type": {"refresh_token"}})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("postToken() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestReadTokenResponseBodyRejectsInvalidBodies(t *testing.T) {
	if _, err := readTokenResponseBody(errorReader{}); err == nil {
		t.Fatal("readTokenResponseBody accepted a reader error")
	}
	if _, err := readTokenResponseBody(strings.NewReader(strings.Repeat("x", (1<<20)+1))); err == nil {
		t.Fatal("readTokenResponseBody accepted an oversized response")
	}
}

func TestExtractAccountIDFallbacks(t *testing.T) {
	claims := []map[string]any{
		{"chatgpt_account_id": "direct"},
		{"organizations": []any{map[string]any{"id": "org"}}},
	}
	for _, claim := range claims {
		if got := ExtractAccountID(jwtForClaims(t, claim)); got == "" {
			t.Fatalf("ExtractAccountID(%#v) returned empty ID", claim)
		}
	}
	for _, token := range []string{"", "a.b", "a.!.c", "a." + base64.RawURLEncoding.EncodeToString([]byte("not-json")) + ".c"} {
		if got := ExtractAccountID(token); got != "" {
			t.Fatalf("ExtractAccountID(%q) = %q", token, got)
		}
	}
}

func TestOAuthPageEscapesContent(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeOAuthPage(recorder, true, `<script>alert(1)</script>`)
	body := recorder.Body.String()
	if !strings.Contains(body, "Authentication Successful") || strings.Contains(body, "<script>") {
		t.Fatalf("unexpected OAuth page %q", body)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func jwtForClaims(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}
