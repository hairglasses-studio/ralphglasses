package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNewWorkflowMarketplace(t *testing.T) {
	m := NewWorkflowMarketplace("https://example.com/index.yaml", "/tmp/wf-test")
	if m == nil {
		t.Fatal("expected non-nil marketplace")
	}
	if m.indexURL != "https://example.com/index.yaml" {
		t.Errorf("unexpected indexURL: %s", m.indexURL)
	}
}

func newTestIndex() []WorkflowEntry {
	return []WorkflowEntry{
		{
			Name:        "deploy-staging",
			Description: "Deploy to staging environment",
			Version:     "1.0.0",
			Author:      "testuser",
			Tags:        []string{"deploy", "staging"},
			Steps:       3,
		},
		{
			Name:        "lint-and-test",
			Description: "Run linters and test suite",
			Version:     "2.1.0",
			Author:      "testuser",
			Tags:        []string{"ci", "test"},
			Steps:       2,
		},
		{
			Name:        "release-prod",
			Description: "Production release pipeline",
			Version:     "1.2.0",
			Author:      "other",
			Tags:        []string{"deploy", "production"},
			Steps:       5,
		},
	}
}

func serveIndex(t *testing.T, entries []WorkflowEntry) *httptest.Server {
	t.Helper()
	data, err := yaml.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal index: %v", err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(data)
	}))
}

func TestSearchAll(t *testing.T) {
	srv := serveIndex(t, newTestIndex())
	defer srv.Close()

	m := NewWorkflowMarketplace(srv.URL, t.TempDir())
	results, err := m.Search("")
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

func TestSearchByName(t *testing.T) {
	srv := serveIndex(t, newTestIndex())
	defer srv.Close()

	m := NewWorkflowMarketplace(srv.URL, t.TempDir())
	results, err := m.Search("deploy")
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 deploy results, got %d", len(results))
	}
}

func TestSearchByTag(t *testing.T) {
	srv := serveIndex(t, newTestIndex())
	defer srv.Close()

	m := NewWorkflowMarketplace(srv.URL, t.TempDir())
	results, err := m.Search("ci")
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 ci result, got %d", len(results))
	}
	if results[0].Name != "lint-and-test" {
		t.Errorf("expected lint-and-test, got %s", results[0].Name)
	}
}

func TestSearchNoMatch(t *testing.T) {
	srv := serveIndex(t, newTestIndex())
	defer srv.Close()

	m := NewWorkflowMarketplace(srv.URL, t.TempDir())
	results, err := m.Search("nonexistent")
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestInstallWithValidation(t *testing.T) {
	// Create a valid workflow YAML.
	wfDef := WorkflowDef{
		Name: "test-workflow",
		Steps: []StepDef{
			{Name: "step1", Command: "echo hello"},
			{Name: "step2", Command: "echo world", DependsOn: []string{"step1"}},
		},
	}
	wfData, err := yaml.Marshal(wfDef)
	if err != nil {
		t.Fatalf("marshal workflow: %v", err)
	}

	hash := sha256.Sum256(wfData)
	checksum := hex.EncodeToString(hash[:])

	index := []WorkflowEntry{
		{
			Name:     "test-workflow",
			Version:  "1.0.0",
			Checksum: checksum,
			Steps:    2,
		},
	}

	// Serve both the index and the workflow artifact.
	mux := http.NewServeMux()
	indexData, _ := yaml.Marshal(index)
	mux.HandleFunc("/index.yaml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(indexData)
	})
	// Set the URL after we know the server address.
	mux.HandleFunc("/workflows/test-workflow.yaml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(wfData)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	index[0].URL = srv.URL + "/workflows/test-workflow.yaml"
	indexData, _ = yaml.Marshal(index)

	// Re-register with updated URL.
	mux.HandleFunc("/index2.yaml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(indexData)
	})

	installDir := t.TempDir()
	m := NewWorkflowMarketplace(srv.URL+"/index2.yaml", installDir)

	if err := m.Install("test-workflow"); err != nil {
		t.Fatalf("install error: %v", err)
	}

	// Verify file was written.
	destPath := filepath.Join(installDir, "test-workflow.yaml")
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read installed file: %v", err)
	}
	if len(data) == 0 {
		t.Error("installed file is empty")
	}
}

func TestInstallChecksumMismatch(t *testing.T) {
	wfDef := WorkflowDef{
		Name:  "bad-checksum",
		Steps: []StepDef{{Name: "step1", Command: "echo hi"}},
	}
	wfData, _ := yaml.Marshal(wfDef)

	index := []WorkflowEntry{
		{
			Name:     "bad-checksum",
			Version:  "1.0.0",
			Checksum: "0000000000000000000000000000000000000000000000000000000000000000",
		},
	}

	mux := http.NewServeMux()
	indexData, _ := yaml.Marshal(index)
	mux.HandleFunc("/index.yaml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(indexData)
	})
	mux.HandleFunc("/wf.yaml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(wfData)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	index[0].URL = srv.URL + "/wf.yaml"
	indexData, _ = yaml.Marshal(index)

	mux.HandleFunc("/index2.yaml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(indexData)
	})

	m := NewWorkflowMarketplace(srv.URL+"/index2.yaml", t.TempDir())
	err := m.Install("bad-checksum")
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
}

func TestInstallNotFound(t *testing.T) {
	srv := serveIndex(t, newTestIndex())
	defer srv.Close()

	m := NewWorkflowMarketplace(srv.URL, t.TempDir())
	err := m.Install("nonexistent-workflow")
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestInstalled(t *testing.T) {
	dir := t.TempDir()

	// Create some fake installed workflows.
	for _, name := range []string{"wf-a.yaml", "wf-b.yaml", "not-yaml.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	m := NewWorkflowMarketplace("", dir)
	installed := m.Installed()

	if len(installed) != 2 {
		t.Errorf("expected 2 installed workflows, got %d", len(installed))
	}
}

func TestInstalledEmptyDir(t *testing.T) {
	m := NewWorkflowMarketplace("", "/nonexistent/path/that/does/not/exist")
	installed := m.Installed()
	if installed != nil {
		t.Errorf("expected nil for nonexistent dir, got %v", installed)
	}
}

func TestFetchIndexCaching(t *testing.T) {
	callCount := 0
	entries := newTestIndex()
	data, _ := yaml.Marshal(entries)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	m := NewWorkflowMarketplace(srv.URL, t.TempDir())

	// First call fetches from server.
	_, err := m.Search("")
	if err != nil {
		t.Fatalf("first search: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 fetch, got %d", callCount)
	}

	// Second call uses cache.
	_, err = m.Search("deploy")
	if err != nil {
		t.Fatalf("second search: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected still 1 fetch (cached), got %d", callCount)
	}
}
