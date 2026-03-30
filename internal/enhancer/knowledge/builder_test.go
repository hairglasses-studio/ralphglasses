package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildFromDir_ModelPackage(t *testing.T) {
	// Find the internal/model package relative to this test
	// Navigate up from internal/enhancer/knowledge to project root
	root := findProjectRoot(t)
	modelDir := filepath.Join(root, "internal", "model")

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		t.Skipf("internal/model directory not found at %s", modelDir)
	}

	g := NewGraph()
	b := NewBuilder(g)
	if err := b.BuildFromDir(modelDir); err != nil {
		t.Fatalf("BuildFromDir failed: %v", err)
	}

	// Should have found the model package
	if g.NodeCount() == 0 {
		t.Fatal("graph has no nodes after scanning internal/model")
	}

	// Check for known types
	repoNode := findNodeByMetadata(g, "name", "Repo")
	if repoNode == nil {
		t.Error("expected to find Repo type")
	}

	loopStatusNode := findNodeByMetadata(g, "name", "LoopStatus")
	if loopStatusNode == nil {
		t.Error("expected to find LoopStatus type")
	}

	// Check for known functions
	loadStatusNode := findNodeByMetadata(g, "name", "LoadStatus")
	if loadStatusNode == nil {
		t.Error("expected to find LoadStatus function")
	}

	// Should have some edges
	if g.EdgeCount() == 0 {
		t.Error("expected some edges")
	}

	t.Logf("Graph: %d nodes, %d edges", g.NodeCount(), g.EdgeCount())
}

func TestBuildFromDir_Synthetic(t *testing.T) {
	// Create a temporary Go package
	dir := t.TempDir()
	writeGoFile(t, dir, "main.go", `package main

import "fmt"

type Config struct {
	Name    string
	Verbose bool
}

func NewConfig(name string) *Config {
	return &Config{Name: name}
}

func (c *Config) Print() {
	fmt.Println(c.Name)
}

func main() {
	cfg := NewConfig("test")
	cfg.Print()
}
`)

	g := NewGraph()
	b := NewBuilder(g)
	if err := b.BuildFromDir(dir); err != nil {
		t.Fatalf("BuildFromDir failed: %v", err)
	}

	// Check package node
	pkgNode := g.GetNode("pkg:main")
	if pkgNode == nil {
		t.Fatal("expected pkg:main node")
	}

	// Check type
	configNode := findNodeByMetadata(g, "name", "Config")
	if configNode == nil {
		t.Fatal("expected Config type")
	}
	if configNode.Kind != KindType {
		t.Errorf("Config.Kind = %q, want %q", configNode.Kind, KindType)
	}
	if !strings.Contains(configNode.Metadata["fields"], "Name") {
		t.Errorf("Config fields should contain Name, got %q", configNode.Metadata["fields"])
	}

	// Check functions
	newConfigNode := findNodeByMetadata(g, "name", "NewConfig")
	if newConfigNode == nil {
		t.Fatal("expected NewConfig function")
	}
	if !strings.Contains(newConfigNode.Metadata["signature"], "NewConfig") {
		t.Errorf("expected signature to contain NewConfig, got %q", newConfigNode.Metadata["signature"])
	}

	// Check method
	printNode := findNodeByMetadata(g, "name", "Print")
	if printNode == nil {
		t.Fatal("expected Print method")
	}
	if printNode.Metadata["receiver"] != "Config" {
		t.Errorf("Print.receiver = %q, want Config", printNode.Metadata["receiver"])
	}

	// Check imports
	fmtNode := g.GetNode("pkg:fmt")
	if fmtNode == nil {
		t.Error("expected fmt import node")
	}

	t.Logf("Synthetic graph: %d nodes, %d edges", g.NodeCount(), g.EdgeCount())
}

func TestBuildFromDir_SkipsTestFiles(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "lib.go", `package lib

func Hello() string { return "hello" }
`)
	writeGoFile(t, dir, "lib_test.go", `package lib

func TestHelper() string { return "test" }
`)

	g := NewGraph()
	b := NewBuilder(g)
	if err := b.BuildFromDir(dir); err != nil {
		t.Fatalf("BuildFromDir failed: %v", err)
	}

	// Should find Hello but not TestHelper
	if findNodeByMetadata(g, "name", "Hello") == nil {
		t.Error("expected Hello function")
	}
	if findNodeByMetadata(g, "name", "TestHelper") != nil {
		t.Error("should not include test file entities")
	}
}

func TestBuildFromDir_SkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "visible.go", `package main

func Visible() {}
`)
	hiddenDir := filepath.Join(dir, ".hidden")
	if err := os.MkdirAll(hiddenDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeGoFile(t, hiddenDir, "hidden.go", `package hidden

func Hidden() {}
`)

	g := NewGraph()
	b := NewBuilder(g)
	if err := b.BuildFromDir(dir); err != nil {
		t.Fatalf("BuildFromDir failed: %v", err)
	}

	if findNodeByMetadata(g, "name", "Visible") == nil {
		t.Error("expected Visible")
	}
	if findNodeByMetadata(g, "name", "Hidden") != nil {
		t.Error("should not include hidden dir entities")
	}
}

func TestBuildFromDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	g := NewGraph()
	b := NewBuilder(g)
	if err := b.BuildFromDir(dir); err != nil {
		t.Fatalf("BuildFromDir on empty dir should not fail: %v", err)
	}
	if g.NodeCount() != 0 {
		t.Errorf("expected 0 nodes for empty dir, got %d", g.NodeCount())
	}
}

func TestBuildFromDir_Interface(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "iface.go", `package iface

type Reader interface {
	Read(p []byte) (n int, err error)
}

type ReadWriter interface {
	Reader
	Write(p []byte) (n int, err error)
}
`)

	g := NewGraph()
	b := NewBuilder(g)
	if err := b.BuildFromDir(dir); err != nil {
		t.Fatalf("BuildFromDir failed: %v", err)
	}

	rn := findNodeByMetadata(g, "name", "Reader")
	if rn == nil {
		t.Fatal("expected Reader interface")
	}
	if rn.Metadata["kind"] != "interface" {
		t.Errorf("Reader.kind = %q, want interface", rn.Metadata["kind"])
	}

	rwn := findNodeByMetadata(g, "name", "ReadWriter")
	if rwn == nil {
		t.Fatal("expected ReadWriter interface")
	}
}

// --- helpers ---

func findProjectRoot(t *testing.T) string {
	t.Helper()
	// Walk up from CWD looking for go.mod
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root (go.mod)")
		}
		dir = parent
	}
}

func findNodeByMetadata(g *Graph, key, value string) *Node {
	for _, n := range g.AllNodes() {
		if n.Metadata[key] == value {
			return &n
		}
	}
	return nil
}

func writeGoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}
