package fontkit

import (
	"context"
	"os"
	"testing"
)

func TestDetect(t *testing.T) {
	status, err := Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Total should be all families
	total := len(status.Installed) + len(status.NotInstalled)
	if total != len(AllFamilies()) {
		t.Errorf("Installed + NotInstalled = %d, want %d", total, len(AllFamilies()))
	}
	// BestFont should always be set (at minimum Menlo)
	if status.BestFont == "" {
		t.Error("BestFont should not be empty")
	}
}

func TestBestAvailable(t *testing.T) {
	// With no fonts installed, should fall back to Menlo
	if got := bestAvailable(nil); got != "Menlo" {
		t.Errorf("bestAvailable(nil) = %q, want Menlo", got)
	}

	// With MonaspiceNe installed, should pick it
	got := bestAvailable([]FontFamily{MonaspiceNe})
	if got != "MonaspiceNeNFM-Regular" {
		t.Errorf("bestAvailable([MonaspiceNe]) = %q, want MonaspiceNeNFM-Regular", got)
	}
}

func TestFontOnDisk_EmptyDirs(t *testing.T) {
	// No directories — should not find any fonts
	if fontOnDisk(nil, MonaspiceNe) {
		t.Error("fontOnDisk with nil dirs should return false")
	}
}

func TestFontOnDisk_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	if fontOnDisk([]string{tmpDir}, MonaspiceNe) {
		t.Error("fontOnDisk with empty dir should return false")
	}
}

func TestFontOnDisk_WithOTFFont(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a fake font file matching the pattern
	fam := MonaspiceNe
	// MonaspiceNe.FilePattern is something like "MonaspiceNe*"
	fakePath := tmpDir + "/MonaspiceNeNFM-Regular.otf"
	os.WriteFile(fakePath, []byte("fake font"), 0644)

	if !fontOnDisk([]string{tmpDir}, fam) {
		t.Error("fontOnDisk should find the OTF font file")
	}
}

func TestFontOnDisk_WithTTFFont(t *testing.T) {
	tmpDir := t.TempDir()
	// The code checks for .ttf variants of .otf patterns
	// Create a fake .ttf file
	fam := MonaspiceNe
	fakePath := tmpDir + "/MonaspiceNeNFM-Regular.ttf"
	os.WriteFile(fakePath, []byte("fake font"), 0644)

	if !fontOnDisk([]string{tmpDir}, fam) {
		t.Error("fontOnDisk should find the TTF font file")
	}
}

func TestFontOnDisk_MultiDirs(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	fam := MonaspiceNe

	// Only in second dir
	fakePath := dir2 + "/MonaspiceNeNFM-Regular.otf"
	os.WriteFile(fakePath, []byte("fake font"), 0644)

	if !fontOnDisk([]string{dir1, dir2}, fam) {
		t.Error("fontOnDisk should find font in second dir")
	}
}

func TestBestAvailable_EmptyInstalledList(t *testing.T) {
	if got := bestAvailable([]FontFamily{}); got != "Menlo" {
		t.Errorf("bestAvailable([]) = %q, want Menlo", got)
	}
}

func TestBrewAvailable(t *testing.T) {
	// Just check it doesn't panic
	_ = brewAvailable(context.Background())
}
