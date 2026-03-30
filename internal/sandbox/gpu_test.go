package sandbox

import (
	"errors"
	"testing"
)

func TestGPUManager_DetectWithDevices(t *testing.T) {
	gm := NewGPUManager()
	gm.detector = func() ([]string, error) {
		return []string{
			"NVIDIA RTX 4090, 25769803776",
			"NVIDIA RTX 4090, 25769803776",
		}, nil
	}

	devices, err := gm.Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2", len(devices))
	}
	if devices[0].Name != "NVIDIA RTX 4090" {
		t.Errorf("device[0].Name = %q, want %q", devices[0].Name, "NVIDIA RTX 4090")
	}
	if devices[0].Memory != 25769803776 {
		t.Errorf("device[0].Memory = %d, want 25769803776", devices[0].Memory)
	}
	if devices[0].ID != "gpu-0" {
		t.Errorf("device[0].ID = %q, want gpu-0", devices[0].ID)
	}
}

func TestGPUManager_DetectNoNvidia(t *testing.T) {
	gm := NewGPUManager()
	gm.detector = func() ([]string, error) {
		return nil, errors.New("no nvidia driver")
	}

	devices, err := gm.Detect()
	if err != nil {
		t.Fatalf("Detect should not error on non-NVIDIA: %v", err)
	}
	if len(devices) != 0 {
		t.Errorf("got %d devices, want 0", len(devices))
	}
}

func TestGPUManager_AllocateRelease(t *testing.T) {
	gm := NewGPUManager()
	gm.detector = func() ([]string, error) {
		return []string{"GPU A, 1024", "GPU B, 2048"}, nil
	}
	gm.Detect()

	// Allocate first device.
	dev1, err := gm.Allocate()
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if dev1.ID != "gpu-0" {
		t.Errorf("got %q, want gpu-0", dev1.ID)
	}

	// Allocate second device.
	dev2, err := gm.Allocate()
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if dev2.ID != "gpu-1" {
		t.Errorf("got %q, want gpu-1", dev2.ID)
	}

	// Third allocate should fail.
	_, err = gm.Allocate()
	if !errors.Is(err, ErrNoGPUAvailable) {
		t.Errorf("expected ErrNoGPUAvailable, got %v", err)
	}

	// Release first, then allocate again.
	if err := gm.Release("gpu-0"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	dev3, err := gm.Allocate()
	if err != nil {
		t.Fatalf("Allocate after release: %v", err)
	}
	if dev3.ID != "gpu-0" {
		t.Errorf("got %q, want gpu-0 (re-allocated)", dev3.ID)
	}
}

func TestGPUManager_DoubleRelease(t *testing.T) {
	gm := NewGPUManager()
	gm.detector = func() ([]string, error) {
		return []string{"GPU A, 1024"}, nil
	}
	gm.Detect()

	dev, _ := gm.Allocate()
	if err := gm.Release(dev.ID); err != nil {
		t.Fatalf("first Release: %v", err)
	}

	err := gm.Release(dev.ID)
	if !errors.Is(err, ErrGPUNotInUse) {
		t.Errorf("expected ErrGPUNotInUse, got %v", err)
	}
}

func TestGPUManager_ReleaseNotFound(t *testing.T) {
	gm := NewGPUManager()
	gm.detector = func() ([]string, error) {
		return []string{"GPU A, 1024"}, nil
	}
	gm.Detect()

	err := gm.Release("gpu-99")
	if !errors.Is(err, ErrGPUNotFound) {
		t.Errorf("expected ErrGPUNotFound, got %v", err)
	}
}

func TestGPUDockerRunArgs(t *testing.T) {
	dev := &GPUDevice{ID: "gpu-0", Name: "RTX 4090"}
	args := DockerRunArgs(dev)

	if len(args) != 4 {
		t.Fatalf("got %d args, want 4: %v", len(args), args)
	}
	if args[0] != "--gpus" {
		t.Errorf("args[0] = %q, want --gpus", args[0])
	}
	if args[1] != "\"device=gpu-0\"" {
		t.Errorf("args[1] = %q, want %q", args[1], "\"device=gpu-0\"")
	}
	if args[2] != "--runtime" {
		t.Errorf("args[2] = %q, want --runtime", args[2])
	}
	if args[3] != "nvidia" {
		t.Errorf("args[3] = %q, want nvidia", args[3])
	}
}

func TestGPUDockerRunArgs_Nil(t *testing.T) {
	args := DockerRunArgs(nil)
	if args != nil {
		t.Errorf("expected nil for nil device, got %v", args)
	}
}

func TestGPUManager_Devices(t *testing.T) {
	gm := NewGPUManager()
	gm.detector = func() ([]string, error) {
		return []string{"GPU A, 1024"}, nil
	}
	gm.Detect()

	devices := gm.Devices()
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}

	// Mutating the returned slice should not affect the manager.
	devices[0].InUse = true
	original := gm.Devices()
	if original[0].InUse {
		t.Error("Devices() returned a reference instead of a copy")
	}
}
