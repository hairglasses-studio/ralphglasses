// Package firecracker provides Firecracker microVM management for sandboxed LLM sessions.
//
//go:build linux

package firecracker

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DefaultBaseImageURL is the default Alpine-based minimal rootfs image URL.
const DefaultBaseImageURL = "https://dl-cdn.alpinelinux.org/alpine/v3.20/releases/x86_64/alpine-minirootfs-3.20.0-x86_64.tar.gz"

// RootFSBuilder constructs ext4 root filesystem images for Firecracker microVMs.
type RootFSBuilder struct {
	// CacheDir is the directory for cached base images (default: ~/.cache/ralphglasses/firecracker).
	CacheDir string
	// BaseImageURL is the URL to download the base rootfs tarball.
	BaseImageURL string
	// SizeMB is the size of the ext4 image in megabytes (default: 1024).
	SizeMB int
	// ExtraFiles maps host paths to guest paths for overlay.
	ExtraFiles map[string]string
	// Logger for build progress.
	Logger *slog.Logger
}

// NewRootFSBuilder returns a builder with sensible defaults.
func NewRootFSBuilder() *RootFSBuilder {
	home, _ := os.UserHomeDir()
	cacheDir := filepath.Join(home, ".cache", "ralphglasses", "firecracker")
	return &RootFSBuilder{
		CacheDir:     cacheDir,
		BaseImageURL: DefaultBaseImageURL,
		SizeMB:       1024,
		ExtraFiles:   make(map[string]string),
		Logger:       slog.Default(),
	}
}

// WithRalphglassesBinary adds the ralphglasses binary to /usr/local/bin/ in the guest.
func (b *RootFSBuilder) WithRalphglassesBinary(hostPath string) *RootFSBuilder {
	b.ExtraFiles[hostPath] = "/usr/local/bin/ralphglasses"
	return b
}

// WithOverlay adds a host file to be placed at the given guest path.
func (b *RootFSBuilder) WithOverlay(hostPath, guestPath string) *RootFSBuilder {
	b.ExtraFiles[hostPath] = guestPath
	return b
}

// Build creates an ext4 rootfs image at the given output path.
// It downloads the base image (if not cached), creates an ext4 filesystem,
// extracts the base image into it, and copies overlay files.
func (b *RootFSBuilder) Build(ctx context.Context, outputPath string) error {
	if err := b.validateOverlayFiles(); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("rootfs: create output dir: %w", err)
	}

	// Download or use cached base image.
	tarball, err := b.ensureBaseImage(ctx)
	if err != nil {
		return fmt.Errorf("rootfs: ensure base image: %w", err)
	}

	// Create empty ext4 image.
	if err := b.createEmptyImage(ctx, outputPath); err != nil {
		return fmt.Errorf("rootfs: create image: %w", err)
	}

	// Mount, extract, overlay.
	if err := b.populateImage(ctx, outputPath, tarball); err != nil {
		return fmt.Errorf("rootfs: populate image: %w", err)
	}

	b.Logger.Info("rootfs built", "path", outputPath, "size_mb", b.SizeMB)
	return nil
}

// ensureBaseImage downloads the base tarball if not already cached.
// Returns the path to the cached tarball.
func (b *RootFSBuilder) ensureBaseImage(ctx context.Context) (string, error) {
	if err := os.MkdirAll(b.CacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}

	// Use SHA256 of URL as cache key.
	h := sha256.Sum256([]byte(b.BaseImageURL))
	cacheKey := fmt.Sprintf("%x", h[:8])

	// Derive extension from URL.
	ext := ".tar.gz"
	if strings.HasSuffix(b.BaseImageURL, ".tar.xz") {
		ext = ".tar.xz"
	}
	cachedPath := filepath.Join(b.CacheDir, "base-"+cacheKey+ext)

	if _, err := os.Stat(cachedPath); err == nil {
		b.Logger.Info("using cached base image", "path", cachedPath)
		return cachedPath, nil
	}

	b.Logger.Info("downloading base image", "url", b.BaseImageURL)
	if err := b.downloadFile(ctx, b.BaseImageURL, cachedPath); err != nil {
		return "", err
	}
	return cachedPath, nil
}

// downloadFile fetches a URL to a local file path.
func (b *RootFSBuilder) downloadFile(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: status %d", url, resp.StatusCode)
	}

	// Write to temp file then rename for atomicity.
	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write download: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}

// createEmptyImage creates an empty ext4 filesystem image.
func (b *RootFSBuilder) createEmptyImage(ctx context.Context, path string) error {
	// Create sparse file with dd.
	sizeStr := fmt.Sprintf("%dM", b.SizeMB)
	dd := exec.CommandContext(ctx, "dd", "if=/dev/zero", "of="+path, "bs=1M", "count=0", "seek="+sizeStr[:len(sizeStr)-1])
	if out, err := dd.CombinedOutput(); err != nil {
		return fmt.Errorf("dd: %w: %s", err, string(out))
	}

	// Format as ext4.
	mkfs := exec.CommandContext(ctx, "mkfs.ext4", "-F", "-q", path)
	if out, err := mkfs.CombinedOutput(); err != nil {
		return fmt.Errorf("mkfs.ext4: %w: %s", err, string(out))
	}

	return nil
}

// populateImage mounts the ext4 image, extracts the base tarball, and copies overlays.
func (b *RootFSBuilder) populateImage(ctx context.Context, imagePath, tarball string) error {
	// Create a temp mount point.
	mountDir, err := os.MkdirTemp("", "rootfs-mount-*")
	if err != nil {
		return fmt.Errorf("create mount dir: %w", err)
	}
	defer os.RemoveAll(mountDir)

	// Mount the image.
	mount := exec.CommandContext(ctx, "mount", "-o", "loop", imagePath, mountDir)
	if out, err := mount.CombinedOutput(); err != nil {
		return fmt.Errorf("mount: %w: %s", err, string(out))
	}
	defer func() {
		umount := exec.CommandContext(context.Background(), "umount", mountDir)
		if out, err := umount.CombinedOutput(); err != nil {
			b.Logger.Error("umount failed", "error", err, "output", string(out))
		}
	}()

	// Extract base tarball.
	tarArgs := []string{"-xf", tarball, "-C", mountDir}
	if strings.HasSuffix(tarball, ".gz") {
		tarArgs = append([]string{"-z"}, tarArgs...)
	} else if strings.HasSuffix(tarball, ".xz") {
		tarArgs = append([]string{"-J"}, tarArgs...)
	}
	tar := exec.CommandContext(ctx, "tar", tarArgs...)
	if out, err := tar.CombinedOutput(); err != nil {
		return fmt.Errorf("extract tarball: %w: %s", err, string(out))
	}

	// Copy overlay files.
	for hostPath, guestPath := range b.ExtraFiles {
		destPath := filepath.Join(mountDir, guestPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("create overlay dir for %s: %w", guestPath, err)
		}
		if err := copyFile(hostPath, destPath); err != nil {
			return fmt.Errorf("copy overlay %s -> %s: %w", hostPath, guestPath, err)
		}
		// Make binaries executable.
		if strings.HasPrefix(guestPath, "/usr/local/bin/") || strings.HasPrefix(guestPath, "/usr/bin/") {
			_ = os.Chmod(destPath, 0o755)
		}
	}

	// Create basic init if none exists.
	initPath := filepath.Join(mountDir, "sbin", "init")
	if _, err := os.Stat(initPath); errors.Is(err, os.ErrNotExist) {
		if err := b.writeDefaultInit(mountDir); err != nil {
			return fmt.Errorf("write default init: %w", err)
		}
	}

	return nil
}

// writeDefaultInit creates a minimal /sbin/init script.
func (b *RootFSBuilder) writeDefaultInit(mountDir string) error {
	initDir := filepath.Join(mountDir, "sbin")
	if err := os.MkdirAll(initDir, 0o755); err != nil {
		return err
	}
	initScript := `#!/bin/sh
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev

# Start SSH if available.
if [ -x /usr/sbin/sshd ]; then
    mkdir -p /run/sshd
    /usr/sbin/sshd
fi

exec /bin/sh
`
	return os.WriteFile(filepath.Join(initDir, "init"), []byte(initScript), 0o755)
}

// validateOverlayFiles checks that all overlay source files exist.
func (b *RootFSBuilder) validateOverlayFiles() error {
	for hostPath, guestPath := range b.ExtraFiles {
		if _, err := os.Stat(hostPath); err != nil {
			return fmt.Errorf("rootfs: overlay source %q (-> %s): %w", hostPath, guestPath, err)
		}
	}
	return nil
}

// copyFile copies a single file from src to dst, preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
