package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// newTestServer returns an httptest.Server that serves update check and
// download endpoints. The artifact content is the provided payload.
func newTestServer(t *testing.T, rel *Release, artifact []byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	checkPath := "/v1/update/" + rel.Channel + "/" + runtime.GOOS + "/" + runtime.GOARCH
	mux.HandleFunc(checkPath, func(w http.ResponseWriter, r *http.Request) {
		cur := r.Header.Get("X-Current-Version")
		if cur == rel.Version {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rel)
	})

	mux.HandleFunc("/artifacts/ralphglasses", func(w http.ResponseWriter, _ *http.Request) {
		w.Write(artifact)
	})

	return httptest.NewServer(mux)
}

func checksum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func TestCheckForUpdate_NewVersion(t *testing.T) {
	artifact := []byte("binary-v2")
	rel := &Release{
		Version:      "2.0.0",
		Checksum:     checksum(artifact),
		ReleaseNotes: "big update",
		Channel:      "stable",
	}
	srv := newTestServer(t, rel, artifact)
	defer srv.Close()
	rel.URL = srv.URL + "/artifacts/ralphglasses"

	u := &Updater{
		Endpoint:       srv.URL,
		CurrentVersion: "1.0.0",
		Channel:        "stable",
	}

	got, err := u.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatalf("CheckForUpdate: %v", err)
	}
	if got == nil {
		t.Fatal("expected release, got nil")
	}
	if got.Version != "2.0.0" {
		t.Errorf("version = %q, want %q", got.Version, "2.0.0")
	}
}

func TestCheckForUpdate_UpToDate(t *testing.T) {
	rel := &Release{
		Version:  "1.0.0",
		URL:      "http://unused",
		Checksum: "unused",
		Channel:  "stable",
	}
	srv := newTestServer(t, rel, nil)
	defer srv.Close()

	u := &Updater{
		Endpoint:       srv.URL,
		CurrentVersion: "1.0.0",
		Channel:        "stable",
	}

	got, err := u.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatalf("CheckForUpdate: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil release for up-to-date, got %+v", got)
	}
}

func TestCheckForUpdate_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	u := &Updater{
		Endpoint:       srv.URL,
		CurrentVersion: "1.0.0",
		Channel:        "stable",
	}

	_, err := u.CheckForUpdate(context.Background())
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestDownload_ValidChecksum(t *testing.T) {
	artifact := []byte("new-binary-content")
	rel := &Release{
		Version:  "2.0.0",
		Checksum: checksum(artifact),
		Channel:  "stable",
	}
	srv := newTestServer(t, rel, artifact)
	defer srv.Close()
	rel.URL = srv.URL + "/artifacts/ralphglasses"

	u := &Updater{
		Endpoint:       srv.URL,
		CurrentVersion: "1.0.0",
		Channel:        "stable",
	}

	path, err := u.Download(context.Background(), rel)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer os.RemoveAll(filepath.Dir(path))

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(data) != string(artifact) {
		t.Errorf("content = %q, want %q", data, artifact)
	}
}

func TestDownload_BadChecksum(t *testing.T) {
	artifact := []byte("new-binary-content")
	rel := &Release{
		Version:  "2.0.0",
		Checksum: "0000000000000000000000000000000000000000000000000000000000000000",
		Channel:  "stable",
	}
	srv := newTestServer(t, rel, artifact)
	defer srv.Close()
	rel.URL = srv.URL + "/artifacts/ralphglasses"

	u := &Updater{
		Endpoint:       srv.URL,
		CurrentVersion: "1.0.0",
		Channel:        "stable",
	}

	_, err := u.Download(context.Background(), rel)
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
}

func TestDownload_NilRelease(t *testing.T) {
	u := &Updater{}
	_, err := u.Download(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil release")
	}
}

func TestApplyAndRollback(t *testing.T) {
	// Create a fake "current binary".
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ralphglasses")
	oldContent := []byte("old-binary")
	if err := os.WriteFile(binPath, oldContent, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a fake "new binary" to apply.
	newContent := []byte("new-binary")
	newPath := filepath.Join(tmpDir, "new-ralphglasses")
	if err := os.WriteFile(newPath, newContent, 0o755); err != nil {
		t.Fatal(err)
	}

	u := &Updater{
		BinaryPath: binPath,
	}

	// Apply
	if err := u.Apply(newPath); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	got, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("read after apply: %v", err)
	}
	if string(got) != string(newContent) {
		t.Errorf("after apply: content = %q, want %q", got, newContent)
	}

	// Rollback
	if err := u.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	got, err = os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("read after rollback: %v", err)
	}
	if string(got) != string(oldContent) {
		t.Errorf("after rollback: content = %q, want %q", got, oldContent)
	}
}

func TestRollback_NoBackup(t *testing.T) {
	u := &Updater{}
	if err := u.Rollback(); err == nil {
		t.Fatal("expected error when no backup exists")
	}
}

func TestCheckURL(t *testing.T) {
	u := &Updater{
		Endpoint: "https://updates.example.com",
		Channel:  "beta",
	}
	want := "https://updates.example.com/v1/update/beta/" + runtime.GOOS + "/" + runtime.GOARCH
	if got := u.checkURL(); got != want {
		t.Errorf("checkURL = %q, want %q", got, want)
	}
}

func TestCheckURL_TrailingSlash(t *testing.T) {
	u := &Updater{
		Endpoint: "https://updates.example.com/",
		Channel:  "stable",
	}
	want := "https://updates.example.com/v1/update/stable/" + runtime.GOOS + "/" + runtime.GOARCH
	if got := u.checkURL(); got != want {
		t.Errorf("checkURL = %q, want %q", got, want)
	}
}
