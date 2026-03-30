// Package sandbox provides container isolation for LLM sessions.
// GPU passthrough support for NVIDIA devices in Docker containers.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

// Sentinel errors for GPU operations.
var (
	ErrNoGPUAvailable = errors.New("no GPU available")
	ErrGPUNotFound    = errors.New("GPU device not found")
	ErrGPUNotInUse    = errors.New("GPU device not in use")
)

// GPUDevice represents a single GPU.
type GPUDevice struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Memory int64  `json:"memory"` // bytes
	InUse  bool   `json:"in_use"`
}

// GPUManager tracks GPU devices and manages allocation for containers.
type GPUManager struct {
	mu      sync.RWMutex
	devices []GPUDevice

	// detector is overridable for testing. Returns raw device info lines.
	detector func() ([]string, error)
}

// NewGPUManager creates a GPUManager with the default NVIDIA detector.
func NewGPUManager() *GPUManager {
	return &GPUManager{
		detector: detectNvidiaDevices,
	}
}

// Detect probes the system for NVIDIA GPUs and populates the device list.
// On non-NVIDIA systems this returns an empty slice and no error.
func (gm *GPUManager) Detect() ([]GPUDevice, error) {
	lines, err := gm.detector()
	if err != nil {
		// No NVIDIA driver is not an error; just means no GPUs.
		return nil, nil
	}

	var devices []GPUDevice
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Expected format from nvidia-smi: "name, memory_bytes"
		// or simple device name lines from /proc/driver/nvidia/gpus/*/information
		parts := strings.SplitN(line, ",", 2)
		name := strings.TrimSpace(parts[0])
		var mem int64
		if len(parts) >= 2 {
			fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &mem)
		}
		devices = append(devices, GPUDevice{
			ID:     fmt.Sprintf("gpu-%d", i),
			Name:   name,
			Memory: mem,
			InUse:  false,
		})
	}

	gm.mu.Lock()
	gm.devices = devices
	gm.mu.Unlock()

	return devices, nil
}

// Allocate picks the first free GPU and marks it as in use.
func (gm *GPUManager) Allocate() (*GPUDevice, error) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	for i := range gm.devices {
		if !gm.devices[i].InUse {
			gm.devices[i].InUse = true
			dev := gm.devices[i] // copy
			return &dev, nil
		}
	}
	return nil, ErrNoGPUAvailable
}

// Release marks a GPU device as free. Returns an error if the device is not
// found or not currently in use.
func (gm *GPUManager) Release(id string) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	for i := range gm.devices {
		if gm.devices[i].ID == id {
			if !gm.devices[i].InUse {
				return fmt.Errorf("%w: %s", ErrGPUNotInUse, id)
			}
			gm.devices[i].InUse = false
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrGPUNotFound, id)
}

// Devices returns a snapshot of all known GPU devices.
func (gm *GPUManager) Devices() []GPUDevice {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	out := make([]GPUDevice, len(gm.devices))
	copy(out, gm.devices)
	return out
}

// DockerRunArgs returns the docker run arguments needed to pass through a GPU.
// Uses the NVIDIA Container Toolkit --gpus flag with the device ID.
func DockerRunArgs(device *GPUDevice) []string {
	if device == nil {
		return nil
	}
	return []string{
		"--gpus", fmt.Sprintf("\"device=%s\"", device.ID),
		"--runtime", "nvidia",
	}
}

// detectNvidiaDevices reads GPU info from /proc/driver/nvidia/gpus/ or falls
// back to nvidia-smi. Returns one line per GPU.
func detectNvidiaDevices() ([]string, error) {
	// Try /proc/driver/nvidia/gpus/ first (Linux with NVIDIA driver).
	entries, err := os.ReadDir("/proc/driver/nvidia/gpus")
	if err == nil && len(entries) > 0 {
		var lines []string
		for _, e := range entries {
			info, err := os.ReadFile(fmt.Sprintf("/proc/driver/nvidia/gpus/%s/information", e.Name()))
			if err != nil {
				continue
			}
			// Parse "Model: <name>" line.
			for _, l := range strings.Split(string(info), "\n") {
				if strings.HasPrefix(l, "Model:") {
					name := strings.TrimSpace(strings.TrimPrefix(l, "Model:"))
					lines = append(lines, name)
					break
				}
			}
		}
		if len(lines) > 0 {
			return lines, nil
		}
	}

	// Fallback: nvidia-smi query.
	cmd := execCommandContext(context.Background(), "nvidia-smi",
		"--query-gpu=name,memory.total", "--format=csv,noheader,nounits")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("no NVIDIA GPUs detected: %w", err)
	}

	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}
