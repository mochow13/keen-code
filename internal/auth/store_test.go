package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestStoreSetGetRemove(t *testing.T) {
	store := NewStoreAt(filepath.Join(t.TempDir(), "auth.json"))
	cred := OAuthCredential{
		Type:         "oauth",
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
		AccountID:    "acct",
	}

	if err := store.Set(OpenAICodexProviderID, cred); err != nil {
		t.Fatalf("Set() failed: %v", err)
	}
	got, ok, err := store.Get(OpenAICodexProviderID)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if !ok {
		t.Fatal("expected stored credential")
	}
	if got.AccessToken != "access" || got.RefreshToken != "refresh" || got.AccountID != "acct" {
		t.Fatalf("unexpected credential: %+v", got)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(store.path)
		if err != nil {
			t.Fatalf("stat auth file: %v", err)
		}
		if got := info.Mode().Perm(); got != 0600 {
			t.Fatalf("expected auth file mode 0600, got %o", got)
		}
	}

	if err := store.Remove(OpenAICodexProviderID); err != nil {
		t.Fatalf("Remove() failed: %v", err)
	}
	if _, ok, err := store.Get(OpenAICodexProviderID); err != nil || ok {
		t.Fatalf("expected removed credential, ok=%v err=%v", ok, err)
	}
}

func TestStoreSetSecuresExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode assertions are not portable on windows")
	}

	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatalf("seed auth file: %v", err)
	}
	store := NewStoreAt(path)

	if err := store.Set(OpenAICodexProviderID, OAuthCredential{
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("Set() failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat auth file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("expected auth file mode 0600, got %o", got)
	}
}

func TestGeneratePKCE(t *testing.T) {
	pkce, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() failed: %v", err)
	}
	if len(pkce.Verifier) < 43 {
		t.Fatalf("expected verifier length >= 43, got %d", len(pkce.Verifier))
	}
	if len(pkce.Challenge) == 0 || pkce.Challenge == pkce.Verifier {
		t.Fatalf("unexpected challenge %q for verifier %q", pkce.Challenge, pkce.Verifier)
	}
}

func TestExtractAccountID(t *testing.T) {
	token := fakeJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "acct_123",
		},
	})
	if got := ExtractAccountID(token); got != "acct_123" {
		t.Fatalf("expected acct_123, got %q", got)
	}
}

func TestValidAccessTokenRefreshPreservesOldRefreshToken(t *testing.T) {
	store := NewStoreAt(filepath.Join(t.TempDir(), "auth.json"))
	if err := store.Set(OpenAICodexProviderID, OAuthCredential{
		Type:         "oauth",
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour),
		AccountID:    "acct",
	}); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "old-refresh" {
			t.Fatalf("unexpected refresh form: %v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "new-access",
			"expires_in":   3600,
		})
	}))
	defer server.Close()

	manager := NewOAuthManager(store)
	manager.Issuer = server.URL
	cred, err := manager.ValidAccessToken(context.Background(), OpenAICodexProviderID)
	if err != nil {
		t.Fatalf("ValidAccessToken() failed: %v", err)
	}
	if cred.AccessToken != "new-access" {
		t.Fatalf("expected refreshed access token, got %q", cred.AccessToken)
	}
	if cred.RefreshToken != "old-refresh" {
		t.Fatalf("expected old refresh token to be preserved, got %q", cred.RefreshToken)
	}
}

func TestPostTokenEmptyBodyReturnsDiagnosticError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	manager := NewOAuthManager(NewStoreAt(filepath.Join(t.TempDir(), "auth.json")))
	manager.Issuer = server.URL

	_, err := manager.postToken(context.Background(), mapValues("grant_type", "refresh_token"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "OpenAI OAuth token response was empty") {
		t.Fatalf("expected empty body diagnostic, got %q", err.Error())
	}
}

func TestPostTokenInvalidJSONReturnsStatusAndContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>not json</html>"))
	}))
	defer server.Close()

	manager := NewOAuthManager(NewStoreAt(filepath.Join(t.TempDir(), "auth.json")))
	manager.Issuer = server.URL

	_, err := manager.postToken(context.Background(), mapValues("grant_type", "refresh_token"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "parse OpenAI OAuth token response") || !strings.Contains(err.Error(), "text/html") {
		t.Fatalf("expected parse diagnostic with content type, got %q", err.Error())
	}
}

func TestPostTokenReadsLargeTokenResponse(t *testing.T) {
	largeToken := strings.Repeat("a", 6000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  largeToken,
			"refresh_token": "refresh",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	manager := NewOAuthManager(NewStoreAt(filepath.Join(t.TempDir(), "auth.json")))
	manager.Issuer = server.URL

	tokens, err := manager.postToken(context.Background(), mapValues("grant_type", "refresh_token"))
	if err != nil {
		t.Fatalf("postToken() failed: %v", err)
	}
	if tokens.AccessToken != largeToken {
		t.Fatalf("expected full large token body to be parsed")
	}
}

func fakeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]any{"alg": "none"})
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func mapValues(key, value string) map[string][]string {
	return map[string][]string{key: {value}}
}

func TestStoreAllReturnsIndependentCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	store := NewStoreAt(path)
	if err := store.Set("provider", OAuthCredential{AccessToken: "token"}); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	all, err := store.All()
	if err != nil {
		t.Fatalf("All() error = %v", err)
	}
	delete(all, "provider")
	if _, ok, err := store.Get("provider"); err != nil || !ok {
		t.Fatalf("mutating All() result changed store: ok=%v err=%v", ok, err)
	}
}

func TestStoreAllRejectsMalformedData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(path, []byte("not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStoreAt(path).All(); err == nil {
		t.Fatal("All() accepted malformed JSON")
	}
}

func TestStoreAllTreatsEmptyDataAsNoCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	for _, data := range [][]byte{nil, []byte("null")} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		all, err := NewStoreAt(path).All()
		if err != nil || len(all) != 0 {
			t.Fatalf("All() = %#v, %v", all, err)
		}
	}
}
