// Package update provides OTA (over-the-air) update functionality for the
// ralphglasses binary. It supports checking for updates, downloading with
// SHA256 verification, atomic binary replacement, and rollback.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Release describes an available update.
type Release struct {
	Version      string `json:"version"`
	URL          string `json:"url"`
	Checksum     string `json:"checksum"` // SHA256 hex
	ReleaseNotes string `json:"release_notes"`
	Channel      string `json:"channel"`
}

// Updater checks for, downloads, and applies OTA updates.
type Updater struct {
	// Endpoint is the base URL for the update server (no trailing slash).
	Endpoint string
	// CurrentVersion is the running binary's version string (semver).
	CurrentVersion string
	// Channel selects the update channel (e.g. "stable", "beta", "nightly").
	Channel string
	// BinaryPath overrides the path to the running binary. When empty the
	// executable path is resolved automatically.
	BinaryPath string

	// HTTPClient is the client used for HTTP requests. If nil http.DefaultClient
	// is used.
	HTTPClient *http.Client

	backupPath string // set after Apply for Rollback
}

// checkURL returns the full URL used to query the update server.
func (u *Updater) checkURL() string {
	return fmt.Sprintf("%s/v1/update/%s/%s/%s",
		strings.TrimRight(u.Endpoint, "/"),
		u.Channel,
		runtime.GOOS,
		runtime.GOARCH,
	)
}

func (u *Updater) httpClient() *http.Client {
	if u.HTTPClient != nil {
		return u.HTTPClient
	}
	return http.DefaultClient
}

// CheckForUpdate queries the update server and returns a Release if a newer
// version is available. It returns nil, nil when the current version is
// already up-to-date.
func (u *Updater) CheckForUpdate(ctx context.Context) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.checkURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("update: build request: %w", err)
	}
	req.Header.Set("X-Current-Version", u.CurrentVersion)

	resp, err := u.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("update: check: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// new version available
	case http.StatusNoContent, http.StatusNotModified:
		return nil, nil // up-to-date
	default:
		return nil, fmt.Errorf("update: server returned %s", resp.Status)
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("update: decode response: %w", err)
	}
	if rel.Version == "" || rel.URL == "" || rel.Checksum == "" {
		return nil, errors.New("update: incomplete release metadata")
	}
	return &rel, nil
}

// Download fetches the release artifact to a temporary directory and verifies
// its SHA256 checksum. It returns the path to the downloaded file.
func (u *Updater) Download(ctx context.Context, release *Release) (string, error) {
	if release == nil {
		return "", errors.New("update: nil release")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, release.URL, nil)
	if err != nil {
		return "", fmt.Errorf("update: build download request: %w", err)
	}

	resp, err := u.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("update: download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("update: download returned %s", resp.Status)
	}

	tmpDir, err := os.MkdirTemp("", "ralphglasses-update-*")
	if err != nil {
		return "", fmt.Errorf("update: create temp dir: %w", err)
	}

	dst := filepath.Join(tmpDir, "ralphglasses")
	f, err := os.Create(dst)
	if err != nil {
		return "", fmt.Errorf("update: create temp file: %w", err)
	}

	h := sha256.New()
	w := io.MultiWriter(f, h)
	if _, err := io.Copy(w, resp.Body); err != nil {
		f.Close()
		return "", fmt.Errorf("update: copy artifact: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("update: close temp file: %w", err)
	}

	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, release.Checksum) {
		os.Remove(dst)
		return "", fmt.Errorf("update: checksum mismatch: got %s, want %s", got, release.Checksum)
	}

	return dst, nil
}

// binaryPath resolves the path to the running binary.
func (u *Updater) binaryPath() (string, error) {
	if u.BinaryPath != "" {
		return u.BinaryPath, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("update: resolve executable: %w", err)
	}
	return filepath.EvalSymlinks(exe)
}

// Apply replaces the running binary with the file at path using atomic
// rename. The old binary is preserved for Rollback. The caller is responsible
// for restarting the process after Apply returns.
func (u *Updater) Apply(path string) error {
	binPath, err := u.binaryPath()
	if err != nil {
		return err
	}

	info, err := os.Stat(binPath)
	if err != nil {
		return fmt.Errorf("update: stat current binary: %w", err)
	}

	// Back up the current binary.
	backupPath := binPath + ".bak." + time.Now().UTC().Format("20060102T150405Z")
	if err := copyFile(binPath, backupPath, info.Mode()); err != nil {
		return fmt.Errorf("update: backup: %w", err)
	}

	// Set executable permissions on the new binary.
	if err := os.Chmod(path, info.Mode()); err != nil {
		return fmt.Errorf("update: chmod new binary: %w", err)
	}

	// Atomic replace via rename.
	if err := os.Rename(path, binPath); err != nil {
		return fmt.Errorf("update: rename: %w", err)
	}

	u.backupPath = backupPath
	return nil
}

// Rollback restores the previous binary version saved by Apply.
func (u *Updater) Rollback() error {
	if u.backupPath == "" {
		return errors.New("update: no backup available for rollback")
	}

	binPath, err := u.binaryPath()
	if err != nil {
		return err
	}

	if err := os.Rename(u.backupPath, binPath); err != nil {
		return fmt.Errorf("update: rollback rename: %w", err)
	}

	u.backupPath = ""
	return nil
}

// copyFile copies src to dst with the given file mode.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
