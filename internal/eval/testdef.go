package eval

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// MetricType enumerates the supported metric categories for A/B test definitions.
type MetricType string

const (
	MetricTypeLatency     MetricType = "latency"
	MetricTypeCost        MetricType = "cost"
	MetricTypeQuality     MetricType = "quality"
	MetricTypeSuccessRate MetricType = "success_rate"
)

// validMetricTypes is the set of recognized metric type strings.
var validMetricTypes = map[MetricType]bool{
	MetricTypeLatency:     true,
	MetricTypeCost:        true,
	MetricTypeQuality:     true,
	MetricTypeSuccessRate: true,
}

// VariantDef describes one side of an A/B test in YAML.
type VariantDef struct {
	Name        string  `yaml:"name" json:"name"`
	Provider    string  `yaml:"provider" json:"provider"`
	Model       string  `yaml:"model" json:"model"`
	Prompt      string  `yaml:"prompt" json:"prompt"`
	Temperature float64 `yaml:"temperature" json:"temperature"`
}

// MetricSpec defines a metric to evaluate in YAML.
type MetricSpec struct {
	Name string     `yaml:"name" json:"name"`
	Type MetricType `yaml:"type" json:"type"`
}

// TestDefinition is the YAML-serializable A/B test definition.
type TestDefinition struct {
	Name        string     `yaml:"name" json:"name"`
	Description string     `yaml:"description,omitempty" json:"description,omitempty"`
	VariantA    VariantDef `yaml:"variant_a" json:"variant_a"`
	VariantB    VariantDef `yaml:"variant_b" json:"variant_b"`
	Metrics     []MetricSpec `yaml:"metrics" json:"metrics"`
	SampleSize  int        `yaml:"sample_size" json:"sample_size"`
	Timeout     string     `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// LoadTestDefinition reads a TestDefinition from a YAML file.
func LoadTestDefinition(path string) (*TestDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read test definition: %w", err)
	}
	return ParseTestDefinition(data)
}

// ParseTestDefinition parses a TestDefinition from raw YAML bytes.
func ParseTestDefinition(data []byte) (*TestDefinition, error) {
	var def TestDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse test definition: %w", err)
	}
	if err := ValidateTestDefinition(&def); err != nil {
		return nil, err
	}
	return &def, nil
}

// SaveTestDefinition writes a TestDefinition to a YAML file.
func SaveTestDefinition(def *TestDefinition, path string) error {
	if err := ValidateTestDefinition(def); err != nil {
		return err
	}
	data, err := yaml.Marshal(def)
	if err != nil {
		return fmt.Errorf("marshal test definition: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// ValidateTestDefinition checks that a TestDefinition is well-formed.
func ValidateTestDefinition(def *TestDefinition) error {
	var errs []string

	if def.Name == "" {
		errs = append(errs, "name is required")
	}

	// Variant A
	if def.VariantA.Provider == "" {
		errs = append(errs, "variant_a.provider is required")
	}
	if def.VariantA.Model == "" {
		errs = append(errs, "variant_a.model is required")
	}

	// Variant B
	if def.VariantB.Provider == "" {
		errs = append(errs, "variant_b.provider is required")
	}
	if def.VariantB.Model == "" {
		errs = append(errs, "variant_b.model is required")
	}

	// Metrics
	if len(def.Metrics) == 0 {
		errs = append(errs, "at least one metric is required")
	}
	for i, m := range def.Metrics {
		if m.Name == "" {
			errs = append(errs, fmt.Sprintf("metrics[%d].name is required", i))
		}
		if !validMetricTypes[m.Type] {
			errs = append(errs, fmt.Sprintf("metrics[%d].type %q is invalid (use: latency, cost, quality, success_rate)", i, m.Type))
		}
	}

	// Sample size
	if def.SampleSize <= 0 {
		errs = append(errs, "sample_size must be > 0")
	}

	// Timeout (optional but must parse if present)
	if def.Timeout != "" {
		if _, err := time.ParseDuration(def.Timeout); err != nil {
			errs = append(errs, fmt.Sprintf("timeout %q is not a valid duration: %v", def.Timeout, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("invalid test definition: %s", strings.Join(errs, "; "))
	}
	return nil
}

// ToABTest converts a TestDefinition to a runtime ABTest.
// Metric evaluators are assigned based on MetricSpec.Type:
//   - latency  → MetricLatency()
//   - cost     → MetricTokenCount() (token proxy)
//   - quality  → MetricQualityScore()
//   - success_rate → MetricOutputLength() (non-empty proxy)
func ToABTest(def *TestDefinition) (*ABTest, error) {
	if err := ValidateTestDefinition(def); err != nil {
		return nil, err
	}

	metrics := make([]MetricDef, len(def.Metrics))
	for i, ms := range def.Metrics {
		md, err := metricDefFromSpec(ms)
		if err != nil {
			return nil, fmt.Errorf("metric %q: %w", ms.Name, err)
		}
		metrics[i] = md
	}

	return &ABTest{
		Name: def.Name,
		VariantA: Config{
			Provider:    def.VariantA.Provider,
			Model:       def.VariantA.Model,
			Prompt:      def.VariantA.Prompt,
			Temperature: def.VariantA.Temperature,
		},
		VariantB: Config{
			Provider:    def.VariantB.Provider,
			Model:       def.VariantB.Model,
			Prompt:      def.VariantB.Prompt,
			Temperature: def.VariantB.Temperature,
		},
		Metrics:    metrics,
		SampleSize: def.SampleSize,
	}, nil
}

// metricDefFromSpec maps a MetricSpec to a runtime MetricDef with the
// appropriate built-in evaluator.
func metricDefFromSpec(ms MetricSpec) (MetricDef, error) {
	switch ms.Type {
	case MetricTypeLatency:
		md := MetricLatency()
		md.Name = ms.Name
		return md, nil
	case MetricTypeCost:
		md := MetricTokenCount()
		md.Name = ms.Name
		return md, nil
	case MetricTypeQuality:
		md := MetricQualityScore()
		md.Name = ms.Name
		return md, nil
	case MetricTypeSuccessRate:
		md := MetricOutputLength()
		md.Name = ms.Name
		return md, nil
	default:
		return MetricDef{}, fmt.Errorf("unsupported metric type: %q", ms.Type)
	}
}
