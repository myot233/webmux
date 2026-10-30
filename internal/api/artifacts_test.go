package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// The folder endpoint keeps the API's authentication and Origin checks,
// creates the folder once, and the folder goes with the removed session.
func TestArtifactsFolderFollowsTheSession(t *testing.T) {
	handler, m, item, _, uploads := uploadAPI(t)
	id := item.Info().ID
	request := func(path, token, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "http://webmux.example"+path, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	path := "/api/sessions/" + id + "/artifacts"
	for name, row := range map[string]struct {
		path, token, origin string
		want                int
	}{
		"missing authentication": {path, "", "", http.StatusUnauthorized},
		"untrusted origin":       {path, "secret", "https://evil.example", http.StatusForbidden},
		"missing session":        {"/api/sessions/missing0/artifacts", "secret", "", http.StatusNotFound},
	} {
		if w := request(row.path, row.token, row.origin); w.Code != row.want {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	if _, err := os.Stat(uploads); !os.IsNotExist(err) {
		t.Fatalf("refused requests created the folder: %v", err)
	}

	var folder struct {
		Path string `json:"path"`
	}
	for range 2 {
		w := request(path, "secret", "")
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &folder) != nil {
			t.Fatalf("ensure: %d %s", w.Code, w.Body.String())
		}
		if info, err := os.Stat(folder.Path); err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Fatalf("folder %q: %v %v", folder.Path, info, err)
		}
	}
	if err := os.WriteFile(folder.Path+"/report.md", []byte("done"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Exiting keeps the folder; removing the session deletes it.
	if err := m.Close(id); err != nil {
		t.Fatal(err)
	}
	<-item.Done()
	if w := request(path, "secret", ""); w.Code != http.StatusOK {
		t.Fatalf("exited session: %d", w.Code)
	}
	r := httptest.NewRequest(http.MethodDelete, "http://webmux.example/api/sessions/"+id+"?remove=1", nil)
	r.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", w.Code, w.Body.String())
	}
	waitAPI(t, "folder removal", func() bool { _, err := os.Stat(folder.Path); return os.IsNotExist(err) })
	if w := request(path, "secret", ""); w.Code != http.StatusNotFound {
		t.Fatalf("removed session: %d", w.Code)
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := os.Stat(folder.Path); !os.IsNotExist(err) {
		t.Fatal("folder recreated for a removed session")
	}
}
