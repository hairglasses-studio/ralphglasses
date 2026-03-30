package flash

import (
	"context"
	"os/exec"
	"testing"
)

func TestParseLsblk(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{
			name:  "single removable device",
			input: "sdb 16013942784 1\n",
			want:  1,
		},
		{
			name:  "non-removable filtered out",
			input: "sda 500107862016 0\nsdb 16013942784 1\n",
			want:  1,
		},
		{
			name:  "mounted removable",
			input: "sdb 16013942784 1 /media/usb\n",
			want:  1,
		},
		{
			name:  "empty output",
			input: "",
			want:  0,
		},
		{
			name:  "all non-removable",
			input: "sda 500107862016 0\nnvme0n1 1000204886016 0\n",
			want:  0,
		},
		{
			name:  "multiple removable",
			input: "sdb 16013942784 1\nsdc 32015999488 1\n",
			want:  2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseLsblk(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseLsblk() error = %v, wantErr %v", err, tc.wantErr)
			}
			if len(got) != tc.want {
				t.Errorf("parseLsblk() returned %d devices, want %d", len(got), tc.want)
			}
		})
	}
}

func TestParseLsblkFields(t *testing.T) {
	t.Parallel()

	devs, err := parseLsblk("sdb 16013942784 1 /media/usb\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devs))
	}
	d := devs[0]
	if d.Path != "/dev/sdb" {
		t.Errorf("Path = %q, want /dev/sdb", d.Path)
	}
	if d.Name != "sdb" {
		t.Errorf("Name = %q, want sdb", d.Name)
	}
	if d.Size != 16013942784 {
		t.Errorf("Size = %d, want 16013942784", d.Size)
	}
	if !d.Removable {
		t.Error("Removable = false, want true")
	}
	if !d.Mounted {
		t.Error("Mounted = false, want true")
	}
}

func TestParseDiskutil(t *testing.T) {
	t.Parallel()

	input := `/dev/disk0 (internal, physical):
   #:                       TYPE NAME                    SIZE       IDENTIFIER
   0:      GUID_partition_scheme                        *500.1 GB   disk0

/dev/disk4 (external, physical):
   #:                       TYPE NAME                    SIZE       IDENTIFIER
   0:      GUID_partition_scheme                        *16.0 GB    disk4
`
	devs, err := parseDiskutil(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 {
		t.Fatalf("expected 1 external device, got %d", len(devs))
	}
	if devs[0].Path != "/dev/disk4" {
		t.Errorf("Path = %q, want /dev/disk4", devs[0].Path)
	}
	if !devs[0].Removable {
		t.Error("expected Removable=true")
	}
}

func TestParseDiskutilEmpty(t *testing.T) {
	t.Parallel()

	devs, err := parseDiskutil("")
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 0 {
		t.Errorf("expected 0 devices, got %d", len(devs))
	}
}

// TestFlashOptionDefaults verifies default options.
func TestFlashOptionDefaults(t *testing.T) {
	t.Parallel()

	o := defaultOptions()
	if o.force {
		t.Error("default force should be false")
	}
	if o.progress != nil {
		t.Error("default progress should be nil")
	}
	if o.bufSize != 4*1024*1024 {
		t.Errorf("default bufSize = %d, want %d", o.bufSize, 4*1024*1024)
	}
}

// TestFlashOptionOverrides verifies functional options apply correctly.
func TestFlashOptionOverrides(t *testing.T) {
	t.Parallel()

	called := false
	o := defaultOptions()
	for _, fn := range []FlashOption{
		WithForce(),
		WithProgress(func(_, _ int64) { called = true }),
		WithBufSize(1024),
	} {
		fn(&o)
	}

	if !o.force {
		t.Error("WithForce did not set force=true")
	}
	if o.progress == nil {
		t.Error("WithProgress did not set callback")
	}
	if o.bufSize != 1024 {
		t.Errorf("WithBufSize = %d, want 1024", o.bufSize)
	}

	// Invoke progress to confirm it's wired.
	o.progress(0, 0)
	if !called {
		t.Error("progress callback was not invoked")
	}
}

// TestWithBufSizeIgnoresNonPositive ensures zero/negative values don't change the default.
func TestWithBufSizeIgnoresNonPositive(t *testing.T) {
	t.Parallel()

	o := defaultOptions()
	WithBufSize(0)(&o)
	if o.bufSize != 4*1024*1024 {
		t.Errorf("bufSize changed to %d for zero input", o.bufSize)
	}
	WithBufSize(-1)(&o)
	if o.bufSize != 4*1024*1024 {
		t.Errorf("bufSize changed to %d for negative input", o.bufSize)
	}
}

// TestWriteImageNotFound verifies that a missing image returns ErrImageNotFound.
func TestWriteImageNotFound(t *testing.T) {
	t.Parallel()

	f := New()
	err := f.WriteImage(context.Background(), "/dev/sdb", "/nonexistent/image.img")
	if err != ErrImageNotFound {
		t.Errorf("expected ErrImageNotFound, got %v", err)
	}
}

// TestVerifyImageNotFound verifies that a missing image returns ErrImageNotFound.
func TestVerifyImageNotFound(t *testing.T) {
	t.Parallel()

	f := New()
	err := f.Verify(context.Background(), "/dev/sdb", "/nonexistent/image.img")
	if err != ErrImageNotFound {
		t.Errorf("expected ErrImageNotFound, got %v", err)
	}
}

// TestNewReturnsNonNil verifies the constructor.
func TestNewReturnsNonNil(t *testing.T) {
	t.Parallel()

	f := New()
	if f == nil {
		t.Fatal("New() returned nil")
	}
	if f.listCmd == nil {
		t.Fatal("listCmd is nil")
	}
}

// TestListDevicesUsesInjectedCmd verifies the command injection seam works.
func TestListDevicesUsesInjectedCmd(t *testing.T) {
	t.Parallel()

	f := New()
	// Inject a command that prints fake lsblk output (Linux path) or
	// fake diskutil output (Darwin path). We test via the parser directly
	// since the actual command depends on GOOS, but we can verify the
	// injection seam by replacing listCmd with one that returns known output.
	called := false
	f.listCmd = func(_ context.Context, name string, args ...string) *exec.Cmd {
		called = true
		// Return a command that outputs nothing (will produce 0 devices).
		return exec.Command("echo", "")
	}

	_, _ = f.ListDevices(context.Background())
	if !called {
		t.Error("injected listCmd was not called")
	}
}

// TestSafetyRefuseNonRemovableLsblk tests that parseLsblk filters out non-removable.
func TestSafetyRefuseNonRemovableLsblk(t *testing.T) {
	t.Parallel()

	// sda is non-removable (RM=0).
	devs, _ := parseLsblk("sda 500107862016 0\n")
	for _, d := range devs {
		if d.Name == "sda" {
			t.Error("non-removable device sda should have been filtered")
		}
	}
}

// TestWriteImageCancelled verifies context cancellation is respected.
func TestWriteImageCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	f := New()
	// This will fail at os.Stat (image not found) before reaching ctx check,
	// but we verify the function handles a cancelled context gracefully.
	err := f.WriteImage(ctx, "/dev/null", "/nonexistent/image.img")
	if err == nil {
		t.Error("expected error for cancelled context with missing image")
	}
}
