package session

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestFindingToTask(t *testing.T) {
	findings := []ScratchpadFinding{
		{
			ID:          "F1",
			Category:    "bug",
			Severity:    "critical",
			Description: "Crash on start",
			Resolved:    false,
		},
		{
			ID:          "F2",
			Category:    "docs",
			Severity:    "low",
			Description: "Missing readme",
			Resolved:    false,
		},
		{
			ID:          "F3",
			Category:    "quality",
			Severity:    "high",
			Description: "Untyped variable",
			Resolved:    true, // should be skipped
		},
	}

	tasks := FindingToTask(findings)

	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}

	// Check bug/critical -> P0
	if tasks[0].ID != "F1" || tasks[0].Type != TaskBugfix || tasks[0].Priority != PriorityP0 {
		t.Errorf("task 0 incorrect: %+v", tasks[0])
	}

	// Check docs/low -> P3
	if tasks[1].ID != "F2" || tasks[1].Type != TaskDocs || tasks[1].Priority != PriorityP3 {
		t.Errorf("task 1 incorrect: %+v", tasks[1])
	}
}

func TestReadFindingsJSONL(t *testing.T) {
	tmpFile := "test_findings.jsonl"
	defer os.Remove(tmpFile)

	finding := ScratchpadFinding{
		ID:          "F1",
		Timestamp:   time.Now().Truncate(time.Second),
		Severity:    "high",
		Category:    "bug",
		Description: "test finding",
	}

	data, _ := json.Marshal(finding)
	_ = os.WriteFile(tmpFile, append(data, '\n'), 0644)

	findings, err := ReadFindingsJSONL(tmpFile)
	if err != nil {
		t.Fatalf("ReadFindingsJSONL failed: %v", err)
	}

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}

	if findings[0].ID != finding.ID || findings[0].Description != finding.Description {
		t.Errorf("finding mismatch: %+v vs %+v", findings[0], finding)
	}
}
