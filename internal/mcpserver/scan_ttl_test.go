package mcpserver

import (
	"context"
	"testing"
	"time"
)

func TestScanTTL_CachesWithinWindow(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.scanTTL = 5 * time.Second

	// Perform initial scan.
	if err := srv.scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	// Within TTL window, reposNil should return false (cache is fresh).
	if srv.reposNil() {
		t.Error("reposNil() = true within TTL window; want false")
	}
}

func TestScanTTL_ExpiredCache(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.scanTTL = 1 * time.Millisecond

	// Perform initial scan.
	if err := srv.scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	// Wait for TTL to expire.
	time.Sleep(5 * time.Millisecond)

	// After TTL expiry, reposNil should return true (cache is stale).
	if !srv.reposNil() {
		t.Error("reposNil() = false after TTL expired; want true")
	}
}

func TestScanTTL_ZeroDisablesTTL(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.scanTTL = 0

	// Perform initial scan.
	if err := srv.scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	// With TTL=0, cache should never expire.
	if srv.reposNil() {
		t.Error("reposNil() = true with scanTTL=0; want false (TTL disabled)")
	}
}

func TestHandleScan_ForcesRefresh(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.scanTTL = 10 * time.Minute // Long TTL, should not matter.

	// Initial scan to populate cache.
	if err := srv.scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	// Record the scan timestamp.
	srv.mu.RLock()
	firstScanAt := srv.lastScanAt
	srv.mu.RUnlock()

	// Small delay to ensure time advances.
	time.Sleep(2 * time.Millisecond)

	// handleScan should always force a fresh scan regardless of TTL.
	result, err := srv.handleScan(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleScan: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleScan returned error: %s", getResultText(result))
	}

	srv.mu.RLock()
	secondScanAt := srv.lastScanAt
	srv.mu.RUnlock()

	if !secondScanAt.After(firstScanAt) {
		t.Errorf("handleScan did not update lastScanAt: first=%v second=%v", firstScanAt, secondScanAt)
	}
}

func TestScanTTL_DefaultValue(t *testing.T) {
	t.Parallel()
	srv := NewServer(t.TempDir())
	if srv.scanTTL != DefaultScanTTL {
		t.Errorf("scanTTL = %v; want %v", srv.scanTTL, DefaultScanTTL)
	}
}
