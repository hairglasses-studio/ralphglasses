package enhancer

import (
	"fmt"
	"testing"
)

func TestScoreDiagnostic(t *testing.T) {
	prompts := []struct {
		name   string
		prompt string
	}{
		{"terrible", "fix this"},
		{"bad", "make it work better"},
		{"lazy", "write some tests"},
		{"mediocre", "You are an expert software engineer. Review this Go function for error handling. Focus on nil checks and unchecked returns. Format output as a numbered list."},
		{"decent", "Analyze the authentication middleware in this codebase. Look for security vulnerabilities, especially around token validation and session management. Provide a severity rating for each finding."},
		{"good_no_xml", "You are a senior Go developer. Write a function that parses a JSON configuration file and returns a strongly-typed Config struct. Handle missing fields with sensible defaults. Include 3 unit tests covering: valid input, missing required fields, and malformed JSON. Use table-driven test style."},
	}

	for _, p := range prompts {
		ar := Analyze(p.prompt)
		fmt.Printf("%-15s  overall=%2d  legacy=%d  grade=%s\n", p.name, ar.ScoreReport.Overall, ar.Score, ar.ScoreReport.Grade)
		for _, d := range ar.ScoreReport.Dimensions {
			fmt.Printf("  %-25s %3d (%s)\n", d.Name, d.Score, d.Grade)
		}
		fmt.Println()
	}
}
