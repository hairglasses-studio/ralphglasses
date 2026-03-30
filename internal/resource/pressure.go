package resource

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PressureLevel represents the severity of resource pressure.
type PressureLevel int

const (
	// PressureLow indicates normal resource usage (<70%).
	PressureLow PressureLevel = iota
	// PressureMedium indicates elevated resource usage (70-85%).
	PressureMedium
	// PressureHigh indicates high resource usage (85-95%).
	PressureHigh
	// PressureCritical indicates critical resource usage (>95%).
	PressureCritical
)

// String returns a human-readable name for the pressure level.
func (p PressureLevel) String() string {
	switch p {
	case PressureLow:
		return "low"
	case PressureMedium:
		return "medium"
	case PressureHigh:
		return "high"
	case PressureCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// ThrottleAction represents the recommended response to resource pressure.
type ThrottleAction int

const (
	// ThrottleNone means no action needed.
	ThrottleNone ThrottleAction = iota
	// ThrottleReduceWorkers means reduce the number of active workers.
	ThrottleReduceWorkers
	// ThrottlePauseLaunches means stop launching new sessions.
	ThrottlePauseLaunches
	// ThrottleEmergencyStop means stop all non-essential work immediately.
	ThrottleEmergencyStop
)

// String returns a human-readable name for the throttle action.
func (t ThrottleAction) String() string {
	switch t {
	case ThrottleNone:
		return "none"
	case ThrottleReduceWorkers:
		return "reduce-workers"
	case ThrottlePauseLaunches:
		return "pause-launches"
	case ThrottleEmergencyStop:
		return "emergency-stop"
	default:
		return "unknown"
	}
}

// PressureReport holds the current pressure assessment for all resource types.
type PressureReport struct {
	CPU            PressureLevel  `json:"cpu"`
	Memory         PressureLevel  `json:"memory"`
	Disk           PressureLevel  `json:"disk"`
	OverallLevel   PressureLevel  `json:"overall_level"`
	Recommendation ThrottleAction `json:"recommendation"`

	// Raw metrics for observability.
	CPULoadPct    float64 `json:"cpu_load_pct"`
	MemoryUsedPct float64 `json:"memory_used_pct"`
	DiskUsedPct   float64 `json:"disk_used_pct"`

	Timestamp time.Time `json:"timestamp"`
}

// Thresholds for pressure level classification (percentages).
const (
	thresholdMedium   = 70.0
	thresholdHigh     = 85.0
	thresholdCritical = 95.0
)

// classifyPressure maps a utilization percentage to a PressureLevel.
func classifyPressure(pct float64) PressureLevel {
	switch {
	case pct >= thresholdCritical:
		return PressureCritical
	case pct >= thresholdHigh:
		return PressureHigh
	case pct >= thresholdMedium:
		return PressureMedium
	default:
		return PressureLow
	}
}

// recommend determines the appropriate ThrottleAction for a given overall pressure level.
func recommend(level PressureLevel) ThrottleAction {
	switch level {
	case PressureCritical:
		return ThrottleEmergencyStop
	case PressureHigh:
		return ThrottlePauseLaunches
	case PressureMedium:
		return ThrottleReduceWorkers
	default:
		return ThrottleNone
	}
}

// maxLevel returns the highest of the given pressure levels.
func maxLevel(levels ...PressureLevel) PressureLevel {
	best := PressureLow
	for _, l := range levels {
		if l > best {
			best = l
		}
	}
	return best
}

// metricsSource abstracts system metric collection for testability.
type metricsSource interface {
	cpuLoadPct() float64
	memoryUsedPct() float64
	diskUsedPct() float64
}

// systemMetrics reads real system metrics.
type systemMetrics struct {
	diskPath string
}

func (s *systemMetrics) cpuLoadPct() float64 {
	loadAvg := readLoadAverage()
	numCPU := float64(runtime.NumCPU())
	if numCPU == 0 {
		return 0
	}
	// Convert load average to percentage of CPU capacity.
	pct := (loadAvg / numCPU) * 100
	if pct > 100 {
		pct = 100
	}
	return pct
}

func (s *systemMetrics) memoryUsedPct() float64 {
	total, avail := readSystemMemory()
	if total == 0 {
		return 0
	}
	return float64(total-avail) / float64(total) * 100
}

func (s *systemMetrics) diskUsedPct() float64 {
	if s.diskPath == "" {
		return 0
	}
	st := Check(s.diskPath)
	return st.DiskUsedPct
}

// readLoadAverage reads the 1-minute load average from the system.
func readLoadAverage() float64 {
	// Try /proc/loadavg first (Linux).
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 0 {
			if v, err := strconv.ParseFloat(fields[0], 64); err == nil {
				return v
			}
		}
	}

	// Fallback: use sysctl on Darwin/BSD.
	if runtime.GOOS == "darwin" || runtime.GOOS == "freebsd" {
		return readLoadAverageSysctl()
	}

	return 0
}

// readSystemMemory returns total and available memory in bytes.
func readSystemMemory() (total, available uint64) {
	// Try /proc/meminfo first (Linux).
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		return parseMeminfo(data)
	}

	// Fallback: use sysctl on Darwin.
	if runtime.GOOS == "darwin" {
		return readMemoryDarwin()
	}

	return 0, 0
}

// parseMeminfo extracts total and available memory from /proc/meminfo content.
func parseMeminfo(data []byte) (total, available uint64) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		// Values in /proc/meminfo are in kB.
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			total = val * 1024
		case strings.HasPrefix(line, "MemAvailable:"):
			available = val * 1024
		}
	}
	return total, available
}

// PressureMonitor polls system resources at a configurable interval and
// reports pressure levels with throttle recommendations.
type PressureMonitor struct {
	interval time.Duration
	source   metricsSource

	mu        sync.RWMutex
	current   PressureReport
	callbacks []func(PressureReport)
	lastLevel PressureLevel

	cancel context.CancelFunc
	done   chan struct{}
}

// NewPressureMonitor creates a PressureMonitor that polls at the given interval.
// diskPath is the filesystem path to monitor for disk pressure.
func NewPressureMonitor(interval time.Duration, diskPath string) *PressureMonitor {
	return &PressureMonitor{
		interval: interval,
		source:   &systemMetrics{diskPath: diskPath},
		done:     make(chan struct{}),
	}
}

// newTestMonitor creates a PressureMonitor with a custom metrics source for testing.
func newTestMonitor(interval time.Duration, src metricsSource) *PressureMonitor {
	return &PressureMonitor{
		interval: interval,
		source:   src,
		done:     make(chan struct{}),
	}
}

// Start begins the polling loop. It blocks until ctx is cancelled or Stop is called.
func (pm *PressureMonitor) Start(ctx context.Context) {
	ctx, pm.cancel = context.WithCancel(ctx)

	// Take an initial reading immediately.
	pm.poll()

	ticker := time.NewTicker(pm.interval)
	defer ticker.Stop()
	defer close(pm.done)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pm.poll()
		}
	}
}

// Stop terminates the polling loop and waits for it to finish.
func (pm *PressureMonitor) Stop() {
	if pm.cancel != nil {
		pm.cancel()
		<-pm.done
	}
}

// CurrentPressure returns the most recent pressure report.
func (pm *PressureMonitor) CurrentPressure() PressureReport {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.current
}

// OnPressureChange registers a callback that fires when the overall pressure
// level transitions (e.g., low -> medium). The callback is NOT fired when the
// level stays the same between polls.
func (pm *PressureMonitor) OnPressureChange(fn func(PressureReport)) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.callbacks = append(pm.callbacks, fn)
}

// poll collects metrics, builds a report, and fires callbacks on transitions.
func (pm *PressureMonitor) poll() {
	cpuPct := pm.source.cpuLoadPct()
	memPct := pm.source.memoryUsedPct()
	diskPct := pm.source.diskUsedPct()

	cpuLevel := classifyPressure(cpuPct)
	memLevel := classifyPressure(memPct)
	diskLevel := classifyPressure(diskPct)
	overall := maxLevel(cpuLevel, memLevel, diskLevel)

	report := PressureReport{
		CPU:            cpuLevel,
		Memory:         memLevel,
		Disk:           diskLevel,
		OverallLevel:   overall,
		Recommendation: recommend(overall),
		CPULoadPct:     cpuPct,
		MemoryUsedPct:  memPct,
		DiskUsedPct:    diskPct,
		Timestamp:      time.Now(),
	}

	pm.mu.Lock()
	pm.current = report
	prevLevel := pm.lastLevel
	pm.lastLevel = overall
	// Copy callbacks slice under lock to avoid holding lock during callbacks.
	cbs := make([]func(PressureReport), len(pm.callbacks))
	copy(cbs, pm.callbacks)
	pm.mu.Unlock()

	// Fire callbacks only on level transitions (skip the very first poll
	// where prevLevel is the zero value and overall might also be zero).
	if overall != prevLevel {
		for _, cb := range cbs {
			cb(report)
		}
	}
}

// Summary returns a human-readable one-line summary of the pressure report.
func (r PressureReport) Summary() string {
	return fmt.Sprintf(
		"pressure=%s cpu=%s(%.0f%%) mem=%s(%.0f%%) disk=%s(%.0f%%) action=%s",
		r.OverallLevel, r.CPU, r.CPULoadPct, r.Memory, r.MemoryUsedPct,
		r.Disk, r.DiskUsedPct, r.Recommendation,
	)
}
