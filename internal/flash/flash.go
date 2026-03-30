package flash

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// BlockDevice represents a block device discovered on the system.
type BlockDevice struct {
	Path      string // e.g. /dev/sdb
	Name      string // e.g. sdb
	Size      uint64 // size in bytes
	Removable bool
	Mounted   bool
}

// ProgressFunc is called during write operations with bytes written so far
// and total bytes expected. Implementations must be safe for concurrent use.
type ProgressFunc func(written, total int64)

// FlashOption configures optional behavior for WriteImage.
type FlashOption func(*flashOptions)

type flashOptions struct {
	force    bool         // allow writing to mounted devices
	progress ProgressFunc // progress callback
	bufSize  int          // copy buffer size in bytes
}

func defaultOptions() flashOptions {
	return flashOptions{
		bufSize: 4 * 1024 * 1024, // 4 MiB
	}
}

// WithForce allows writing to mounted devices. Without this option,
// WriteImage returns an error if the target device is mounted.
func WithForce() FlashOption {
	return func(o *flashOptions) { o.force = true }
}

// WithProgress sets a callback that receives periodic progress updates.
func WithProgress(fn ProgressFunc) FlashOption {
	return func(o *flashOptions) { o.progress = fn }
}

// WithBufSize sets the copy buffer size in bytes. Defaults to 4 MiB.
func WithBufSize(size int) FlashOption {
	return func(o *flashOptions) {
		if size > 0 {
			o.bufSize = size
		}
	}
}

// Flash provides USB image writing with safety checks.
type Flash struct {
	// listCmd overrides the command runner for listing devices (testing seam).
	listCmd func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// New returns a Flash instance with default settings.
func New() *Flash {
	return &Flash{
		listCmd: exec.CommandContext,
	}
}

// ListDevices enumerates removable block devices. On Linux it uses lsblk;
// on macOS it uses diskutil. Other platforms return an unsupported error.
func (f *Flash) ListDevices(ctx context.Context) ([]BlockDevice, error) {
	switch runtime.GOOS {
	case "linux":
		return f.listDevicesLinux(ctx)
	case "darwin":
		return f.listDevicesDarwin(ctx)
	default:
		return nil, fmt.Errorf("flash: unsupported platform %s", runtime.GOOS)
	}
}

// listDevicesLinux parses lsblk JSON-ish output to find removable devices.
func (f *Flash) listDevicesLinux(ctx context.Context) ([]BlockDevice, error) {
	cmd := f.listCmd(ctx, "lsblk", "-dno", "NAME,SIZE,RM,MOUNTPOINT", "--bytes")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("flash: lsblk: %w", err)
	}
	return parseLsblk(string(out))
}

// parseLsblk parses the columnar output from lsblk -dno NAME,SIZE,RM,MOUNTPOINT --bytes.
func parseLsblk(output string) ([]BlockDevice, error) {
	var devices []BlockDevice
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		name := fields[0]
		size, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		removable := fields[2] == "1"
		mounted := len(fields) >= 4 && fields[3] != ""

		if !removable {
			continue
		}
		devices = append(devices, BlockDevice{
			Path:      "/dev/" + name,
			Name:      name,
			Size:      size,
			Removable: removable,
			Mounted:   mounted,
		})
	}
	return devices, nil
}

// listDevicesDarwin uses diskutil to find external/removable disks on macOS.
func (f *Flash) listDevicesDarwin(ctx context.Context) ([]BlockDevice, error) {
	cmd := f.listCmd(ctx, "diskutil", "list", "-plist", "external")
	out, err := cmd.Output()
	if err != nil {
		// No external disks is not an error — diskutil exits non-zero.
		if len(out) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("flash: diskutil: %w", err)
	}
	// Simplified parsing: look for /dev/disk identifiers.
	// Full plist parsing would require encoding/xml; we keep stdlib-only.
	return parseDiskutil(string(out))
}

// parseDiskutil does a best-effort extraction of disk identifiers from
// diskutil list output. It returns devices marked as external/removable.
func parseDiskutil(output string) ([]BlockDevice, error) {
	var devices []BlockDevice
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "/dev/disk") {
			continue
		}
		// Lines look like: /dev/disk4 (external, physical):
		parts := strings.Fields(line)
		if len(parts) < 1 {
			continue
		}
		path := parts[0]
		name := strings.TrimPrefix(path, "/dev/")
		external := strings.Contains(line, "external")
		if !external {
			continue
		}
		devices = append(devices, BlockDevice{
			Path:      path,
			Name:      name,
			Removable: true,
			Mounted:   false, // would need per-partition check
		})
	}
	return devices, nil
}

// ErrNotRemovable is returned when attempting to write to a non-removable device.
var ErrNotRemovable = errors.New("flash: device is not removable — refusing to write")

// ErrMounted is returned when the target device is mounted and --force was not given.
var ErrMounted = errors.New("flash: device is mounted — use WithForce() to override")

// ErrImageNotFound is returned when the image file does not exist.
var ErrImageNotFound = errors.New("flash: image file not found")

// WriteImage writes imagePath to the block device at devicePath.
// It performs safety checks before writing:
//   - The device must be removable (or WriteImage returns ErrNotRemovable).
//   - The device must not be mounted unless WithForce() is provided.
//   - The image file must exist.
//
// The caller is responsible for ensuring adequate permissions (typically root).
func (f *Flash) WriteImage(ctx context.Context, devicePath, imagePath string, opts ...FlashOption) error {
	o := defaultOptions()
	for _, fn := range opts {
		fn(&o)
	}

	// Validate image exists.
	imgInfo, err := os.Stat(imagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrImageNotFound
		}
		return fmt.Errorf("flash: stat image: %w", err)
	}

	// Safety: check that the device is removable.
	removable, mounted, err := f.checkDevice(ctx, devicePath)
	if err != nil {
		return err
	}
	if !removable {
		return ErrNotRemovable
	}
	if mounted && !o.force {
		return ErrMounted
	}

	// Open image for reading.
	imgFile, err := os.Open(imagePath)
	if err != nil {
		return fmt.Errorf("flash: open image: %w", err)
	}
	defer imgFile.Close()

	// Open device for writing.
	devFile, err := os.OpenFile(devicePath, os.O_WRONLY|os.O_SYNC, 0)
	if err != nil {
		return fmt.Errorf("flash: open device: %w", err)
	}
	defer devFile.Close()

	total := imgInfo.Size()
	buf := make([]byte, o.bufSize)
	var written int64

	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("flash: cancelled: %w", err)
		}

		n, readErr := imgFile.Read(buf)
		if n > 0 {
			nw, writeErr := devFile.Write(buf[:n])
			if writeErr != nil {
				return fmt.Errorf("flash: write: %w", writeErr)
			}
			written += int64(nw)
			if o.progress != nil {
				o.progress(written, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("flash: read: %w", readErr)
		}
	}

	// Sync to ensure data is flushed to the device.
	if err := devFile.Sync(); err != nil {
		return fmt.Errorf("flash: sync: %w", err)
	}

	return nil
}

// Verify reads the device and compares a SHA-256 checksum against the image file.
// It reads only as many bytes as the image contains.
func (f *Flash) Verify(ctx context.Context, devicePath, imagePath string) error {
	imgInfo, err := os.Stat(imagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrImageNotFound
		}
		return fmt.Errorf("flash: stat image: %w", err)
	}

	imgHash, err := hashFile(ctx, imagePath, imgInfo.Size())
	if err != nil {
		return fmt.Errorf("flash: hash image: %w", err)
	}

	devHash, err := hashFile(ctx, devicePath, imgInfo.Size())
	if err != nil {
		return fmt.Errorf("flash: hash device: %w", err)
	}

	if imgHash != devHash {
		return fmt.Errorf("flash: verification failed: image=%s device=%s", imgHash, devHash)
	}
	return nil
}

// hashFile computes SHA-256 of the first limit bytes of the file at path.
func hashFile(ctx context.Context, path string, limit int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	reader := io.LimitReader(f, limit)
	buf := make([]byte, 4*1024*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, readErr := reader.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// checkDevice determines whether a device path refers to a removable and/or
// mounted device. On Linux it reads sysfs; on macOS it shells out to diskutil.
func (f *Flash) checkDevice(ctx context.Context, devicePath string) (removable, mounted bool, err error) {
	switch runtime.GOOS {
	case "linux":
		return f.checkDeviceLinux(devicePath)
	case "darwin":
		return f.checkDeviceDarwin(ctx, devicePath)
	default:
		return false, false, fmt.Errorf("flash: unsupported platform %s", runtime.GOOS)
	}
}

// checkDeviceLinux reads /sys/block/<name>/removable and checks /proc/mounts.
func (f *Flash) checkDeviceLinux(devicePath string) (removable, mounted bool, err error) {
	name := strings.TrimPrefix(devicePath, "/dev/")
	if name == "" {
		return false, false, fmt.Errorf("flash: invalid device path %q", devicePath)
	}

	// Check removable flag in sysfs.
	data, err := os.ReadFile("/sys/block/" + name + "/removable")
	if err != nil {
		return false, false, fmt.Errorf("flash: read sysfs: %w", err)
	}
	removable = strings.TrimSpace(string(data)) == "1"

	// Check mount status from /proc/mounts.
	mounts, err := os.ReadFile("/proc/mounts")
	if err != nil {
		// /proc/mounts may not exist in containers; don't fail.
		return removable, false, nil
	}
	for _, line := range strings.Split(string(mounts), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 1 && strings.HasPrefix(fields[0], devicePath) {
			mounted = true
			break
		}
	}
	return removable, mounted, nil
}

// checkDeviceDarwin uses diskutil info to determine removable/mount status.
func (f *Flash) checkDeviceDarwin(ctx context.Context, devicePath string) (removable, mounted bool, err error) {
	cmd := f.listCmd(ctx, "diskutil", "info", devicePath)
	out, err := cmd.Output()
	if err != nil {
		return false, false, fmt.Errorf("flash: diskutil info: %w", err)
	}
	output := string(out)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Removable Media:") {
			removable = strings.Contains(line, "Removable")
		}
		if strings.HasPrefix(line, "Mounted:") {
			mounted = strings.Contains(line, "Yes")
		}
		// Also accept "Ejectable" as a proxy for removable USB drives.
		if strings.HasPrefix(line, "Ejectable:") && strings.Contains(line, "Yes") {
			removable = true
		}
	}
	return removable, mounted, nil
}
