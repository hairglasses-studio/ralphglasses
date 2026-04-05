package workflow

import (
	"fmt"
	"strconv"
	"strings"
)

// EvalCondition evaluates a condition string against workflow step results.
// Supported expressions:
//   - "step_name.status == succeeded"  — check step status
//   - "step_name.status != failed"     — negative check
//   - "step_name.output contains X"    — output substring check
//   - "always"                         — always true (explicit unconditional)
//   - "never"                          — always false (skip)
//   - ""                               — empty condition is always true
//
// Returns true if the condition is met and the step should execute.
func EvalCondition(condition string, results map[string]StepResult) bool {
	condition = strings.TrimSpace(condition)
	if condition == "" || condition == "always" {
		return true
	}
	if condition == "never" {
		return false
	}

	// Handle AND chains: "step_a.status == succeeded && step_b.status == succeeded"
	if strings.Contains(condition, "&&") {
		parts := strings.SplitSeq(condition, "&&")
		for part := range parts {
			if !evalSingleCondition(strings.TrimSpace(part), results) {
				return false
			}
		}
		return true
	}

	// Handle OR chains: "step_a.status == succeeded || step_b.status == succeeded"
	if strings.Contains(condition, "||") {
		parts := strings.SplitSeq(condition, "||")
		for part := range parts {
			if evalSingleCondition(strings.TrimSpace(part), results) {
				return true
			}
		}
		return false
	}

	return evalSingleCondition(condition, results)
}

// evalSingleCondition evaluates a single condition expression.
func evalSingleCondition(expr string, results map[string]StepResult) bool {
	// Pattern: "step_name.field op value"
	parts := tokenize(expr)
	if len(parts) < 3 {
		return false
	}

	lhs := parts[0]
	op := parts[1]
	rhs := strings.Join(parts[2:], " ") // value may have spaces

	// Resolve left-hand side: "step_name.field"
	lhsVal := resolveRef(lhs, results)

	switch op {
	case "==":
		return lhsVal == rhs
	case "!=":
		return lhsVal != rhs
	case "contains":
		return strings.Contains(lhsVal, rhs)
	case ">":
		return compareNumeric(lhsVal, rhs) > 0
	case "<":
		return compareNumeric(lhsVal, rhs) < 0
	case ">=":
		return compareNumeric(lhsVal, rhs) >= 0
	case "<=":
		return compareNumeric(lhsVal, rhs) <= 0
	default:
		return false
	}
}

// resolveRef resolves a dotted reference like "step_name.status" or "step_name.output"
// against the results map.
func resolveRef(ref string, results map[string]StepResult) string {
	parts := strings.SplitN(ref, ".", 2)
	if len(parts) != 2 {
		return ref // not a reference, return as literal
	}

	stepName := parts[0]
	field := parts[1]

	result, ok := results[stepName]
	if !ok {
		return ""
	}

	switch field {
	case "status":
		return string(result.Status)
	case "output":
		return result.Output
	case "error":
		return result.Error
	case "retries":
		return fmt.Sprintf("%d", result.Retries)
	case "duration_ms":
		return fmt.Sprintf("%d", result.Duration.Milliseconds())
	default:
		return ""
	}
}

// tokenize splits a condition expression into tokens, respecting quoted strings.
func tokenize(s string) []string {
	s = strings.TrimSpace(s)
	var tokens []string
	var current strings.Builder
	inQuote := false

	for _, ch := range s {
		switch {
		case ch == '"' || ch == '\'':
			inQuote = !inQuote
		case ch == ' ' && !inQuote:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(ch)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// compareNumeric compares two string values as numbers.
// Returns -1, 0, or 1. Non-numeric values compare as 0.
func compareNumeric(a, b string) int {
	fa, errA := strconv.ParseFloat(a, 64)
	fb, errB := strconv.ParseFloat(b, 64)
	if errA != nil || errB != nil {
		return 0
	}
	if fa < fb {
		return -1
	}
	if fa > fb {
		return 1
	}
	return 0
}
