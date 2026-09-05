package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panhui/tz/internal/store"
)

func TestUnrestrictedAdminTokens(t *testing.T) {
	t.Setenv("TZ_ENV_FILE", "")
	path := filepath.Join(t.TempDir(), "data.json")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	app := &server{store: s, adminToken: "initial"}
	for _, token := range []string{"1", " 中文 !\"'\\$` ", strings.Repeat("a", 200), "line1\nline2"} {
		body, _ := json.Marshal(map[string]string{"token": token})
		req := httptest.NewRequest(http.MethodPut, "/api/admin-token", bytes.NewReader(body))
		req.Header.Set("X-TZ-Admin-Token", base64.StdEncoding.EncodeToString([]byte(app.adminToken)))
		w := httptest.NewRecorder()
		app.api(w, req)
		if w.Code != 200 {
			t.Fatalf("token rejected: %s", w.Body.String())
		}
		req = httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
		req.Header.Set("X-TZ-Admin-Token", base64.StdEncoding.EncodeToString([]byte(token)))
		if !app.authorized(req) {
			t.Fatal("saved token cannot authenticate")
		}
		reopened, err := store.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		persisted, err := reopened.EnsureAdminToken("initial")
		if err != nil || persisted != token {
			t.Fatal("token did not survive restart")
		}
	}
	if validAdminToken("") {
		t.Fatal("empty authentication must not be enabled")
	}
}

func TestTokenEnvEscapingAndRepeatedUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.env")
	if err := os.WriteFile(path, []byte("TZ_ADMIN_TOKEN=old\nTZ_LISTEN=:876\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{" 中文 !\"'\\$` ", "first\nTZ_LISTEN=:999\nlast", "1"} {
		if err := persistAdminTokenEnv(path, token); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		want := "TZ_ADMIN_TOKEN=" + quoteEnvToken(token) + "\nTZ_LISTEN=:876\n"
		if string(b) != want {
			t.Fatalf("unexpected environment file: %q", b)
		}
	}
}

func TestDeleteQueuesUninstallAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AutoReport("node-12345678", "", "192.0.2.1", "v0.6.0", store.Metrics{}); err != nil {
		t.Fatal(err)
	}
	legacy, err := s.CreateNode("legacy", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	app := &server{store: s, adminToken: "admin", agentToken: "shared"}
	for _, id := range []string{"node-12345678", legacy.ID} {
		req := httptest.NewRequest(http.MethodDelete, "/api/nodes/"+id, nil)
		req.Header.Set("Authorization", "Bearer admin")
		w := httptest.NewRecorder()
		app.api(w, req)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	app.store, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		token, id          string
		supported          bool
		status             int
		uninstall, upgrade bool
	}{
		{"shared", "node-12345678", false, 200, false, true},
		{"shared", "node-12345678", true, 200, true, false},
		{"shared", "node-12345678", true, 200, true, false},
		{legacy.AgentToken, "legacy-new-id", true, 200, true, false},
		{"wrong", "node-12345678", true, 401, false, false},
	} {
		body, _ := json.Marshal(map[string]any{"nodeId": tc.id, "uninstallSupported": tc.supported})
		req := httptest.NewRequest(http.MethodPost, "/api/report", bytes.NewReader(body))
		req.Header.Set("X-Agent-Token", tc.token)
		w := httptest.NewRecorder()
		app.api(w, req)
		var got struct{ Uninstall, Upgrade bool }
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if w.Code != tc.status || got.Uninstall != tc.uninstall || got.Upgrade != tc.upgrade {
			t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
		}
	}
	_, nodes := app.store.Snapshot()
	if len(nodes) != 0 {
		t.Fatal("deleted node re-enrolled")
	}
}
