package telemetry

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTelemetryEnvironmentSetting(t *testing.T) {
	tests := []struct {
		name, value  string
		set, enabled bool
	}{
		{name: "unset"},
		{name: "enabled", value: " YES ", set: true, enabled: true},
		{name: "disabled", value: "off", set: true},
		{name: "invalid", value: "maybe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "unset" {
				t.Setenv(telemetryEnvVar, "")
			} else {
				t.Setenv(telemetryEnvVar, tt.value)
			}
			enabled, set := telemetryEnvironmentSetting()
			if enabled != tt.enabled || set != tt.set {
				t.Fatalf("telemetryEnvironmentSetting() = (%v, %v), want (%v, %v)", enabled, set, tt.enabled, tt.set)
			}
		})
	}
}

func TestEnvDisabled(t *testing.T) {
	for _, tt := range []struct {
		value string
		want  bool
	}{{"", false}, {"0", false}, {"false", false}, {"off", false}, {"no", false}, {"1", true}, {"yes", true}} {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv("TEST_DISABLED", tt.value)
			if got := envDisabled("TEST_DISABLED"); got != tt.want {
				t.Fatalf("envDisabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnabled(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "")
	t.Setenv("CI", "")
	t.Setenv(telemetryEnvVar, "true")
	if !Enabled("measurement", "secret") {
		t.Fatal("expected telemetry enabled")
	}
	if Enabled("", "secret") || Enabled("measurement", " ") {
		t.Fatal("expected missing configuration to disable telemetry")
	}
	t.Setenv("CI", "1")
	if Enabled("measurement", "secret") {
		t.Fatal("expected CI to disable telemetry")
	}
}

func TestNew(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "")
	t.Setenv("CI", "")
	t.Setenv(telemetryEnvVar, "true")
	t.Setenv("HOME", t.TempDir())

	oldClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = oldClient })
	sent := make(chan struct{}, 2)
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		sent <- struct{}{}
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}

	if reporter := New(Config{}); reporter != nil {
		t.Fatal("New() returned a reporter with telemetry disabled")
	}

	cfg := Config{MeasurementID: "mid", APISecret: "secret", Version: "1.0", Mode: "interactive"}
	reporter := New(cfg)
	if reporter == nil {
		t.Fatal("New() returned nil with telemetry enabled")
	}
	if reporter.config != cfg || reporter.clientID == "" || reporter.sessionID == "" || reporter.startedAt.IsZero() {
		t.Fatalf("New() returned unexpected reporter %#v", reporter)
	}
	<-sent
	reporter.Close()
	<-sent
}

func TestStateRoundTripAndClientIDReuse(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := saveState(state{ClientID: "existing"}); err != nil {
		t.Fatalf("saveState() error = %v", err)
	}
	loaded, err := loadState()
	if err != nil || loaded.ClientID != "existing" {
		t.Fatalf("loadState() = %#v, %v", loaded, err)
	}
	id, err := loadOrCreateClientID()
	if err != nil || id != "existing" {
		t.Fatalf("loadOrCreateClientID() = %q, %v", id, err)
	}
}

func TestLoadOrCreateClientIDCreatesUUID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	id, err := loadOrCreateClientID()
	if err != nil {
		t.Fatalf("loadOrCreateClientID() error = %v", err)
	}
	if len(id) != 36 || id[14] != '4' {
		t.Fatalf("unexpected client ID %q", id)
	}
	loaded, err := loadState()
	if err != nil || loaded.ClientID != id {
		t.Fatalf("persisted state = %#v, %v", loaded, err)
	}
}

func TestLoadStateRejectsInvalidJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Dir(statePath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath(), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadState(); err == nil {
		t.Fatal("loadState() accepted invalid JSON")
	}
}

func TestStatePathAndRandomID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got, want := statePath(), filepath.Join(home, ".keen", "telemetry.json"); got != want {
		t.Fatalf("statePath() = %q, want %q", got, want)
	}
	id, err := randomID()
	if err != nil {
		t.Fatalf("randomID() error = %v", err)
	}
	if len(id) != 36 || id[14] != '4' || id[19] != '8' && id[19] != '9' && id[19] != 'a' && id[19] != 'b' {
		t.Fatalf("randomID() = %q, not an RFC 4122 version 4 UUID", id)
	}
}

func TestReporterNewEventAndSend(t *testing.T) {
	oldClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = oldClient })
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Query().Get("measurement_id") != "mid" || req.URL.Query().Get("api_secret") != "secret" {
			t.Fatalf("unexpected request %s %s", req.Method, req.URL)
		}
		var got payload
		if err := json.NewDecoder(req.Body).Decode(&got); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if got.ClientID != "client" || len(got.Events) != 1 || got.Events[0].Name != "event" {
			t.Fatalf("unexpected payload %#v", got)
		}
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})
	http.DefaultClient = &http.Client{Transport: transport}
	r := &Reporter{clientID: "client", sessionID: "session", config: Config{MeasurementID: "mid", APISecret: "secret", Version: "1.0", Mode: "build"}}
	item := r.newEvent("event", map[string]any{})
	if item.Params["session_id"] != "session" || item.Params["keen_version"] != "1.0" || item.Params["mode"] != "build" {
		t.Fatalf("unexpected event params %#v", item.Params)
	}
	if err := r.send(context.Background(), item); err != nil {
		t.Fatalf("send() error = %v", err)
	}
}

func TestReporterSendRejectsErrorStatus(t *testing.T) {
	oldClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = oldClient })
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	r := &Reporter{clientID: "client", config: Config{MeasurementID: "mid", APISecret: "secret"}}
	if err := r.send(context.Background(), event{Name: "event", Params: map[string]any{}}); err == nil {
		t.Fatal("expected status error")
	}
}

func TestReporterCloseNilAndReporter(t *testing.T) {
	var nilReporter *Reporter
	nilReporter.Close()
	oldClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = oldClient })
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	(&Reporter{clientID: "client", sessionID: "session", startedAt: time.Now(), config: Config{MeasurementID: "mid", APISecret: "secret"}}).Close()
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
