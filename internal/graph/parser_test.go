package graph

import (
	"strings"
	"testing"
)

func TestNewCodeParser(t *testing.T) {
	p := NewCodeParser()
	if p == nil {
		t.Fatal("expected non-nil parser")
	}
}

func TestParseSource_InvalidSyntax(t *testing.T) {
	p := NewCodeParser()
	store := NewGraphStore()
	err := p.ParseSource(store, "bad.go", `not valid go`)
	if err == nil {
		t.Fatal("expected error for invalid Go source")
	}
	if !strings.Contains(err.Error(), "graph: parse source") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestParseFile_NonExistent(t *testing.T) {
	p := NewCodeParser()
	store := NewGraphStore()
	err := p.ParseFile(store, "/nonexistent/file.go")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "graph: parse") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestParseDir_NonExistent(t *testing.T) {
	p := NewCodeParser()
	store := NewGraphStore()
	err := p.ParseDir(store, "/nonexistent/dir")
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
	if !strings.Contains(err.Error(), "graph: parse dir") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestParseSource_PackageNode(t *testing.T) {
	p := NewCodeParser()
	store := NewGraphStore()
	src := `package mypkg`
	if err := p.ParseSource(store, "mypkg.go", src); err != nil {
		t.Fatalf("parse: %v", err)
	}
	n := store.GetNode("pkg:mypkg")
	if n == nil {
		t.Fatal("missing package node")
	}
	if n.Kind != KindPackage {
		t.Fatalf("expected package kind, got %s", n.Kind)
	}
	if n.Name != "mypkg" {
		t.Fatalf("expected name mypkg, got %s", n.Name)
	}
}

func TestParseSource_FileNode(t *testing.T) {
	p := NewCodeParser()
	store := NewGraphStore()
	src := `package mypkg`
	if err := p.ParseSource(store, "mypkg.go", src); err != nil {
		t.Fatalf("parse: %v", err)
	}
	// File node should exist and be declared in the package.
	nodes := store.NodesByKind(KindFile)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 file node, got %d", len(nodes))
	}
	if nodes[0].Name != "mypkg.go" {
		t.Fatalf("expected file name mypkg.go, got %s", nodes[0].Name)
	}
	if nodes[0].Package != "mypkg" {
		t.Fatalf("expected package mypkg, got %s", nodes[0].Package)
	}
}

func TestParseSource_Imports(t *testing.T) {
	p := NewCodeParser()
	store := NewGraphStore()
	src := `package mypkg

import (
	"fmt"
	"strings"
)
`
	if err := p.ParseSource(store, "mypkg.go", src); err != nil {
		t.Fatalf("parse: %v", err)
	}

	fmtNode := store.GetNode("pkg:fmt")
	if fmtNode == nil {
		t.Fatal("missing fmt import node")
	}
	stringsNode := store.GetNode("pkg:strings")
	if stringsNode == nil {
		t.Fatal("missing strings import node")
	}

	impEdges := store.EdgesByKind(EdgeImports)
	if len(impEdges) != 2 {
		t.Fatalf("expected 2 import edges, got %d", len(impEdges))
	}
}

func TestParseSource_Function(t *testing.T) {
	p := NewCodeParser()
	store := NewGraphStore()
	src := `package mypkg

func DoWork() {}
func helper() {}
`
	if err := p.ParseSource(store, "mypkg.go", src); err != nil {
		t.Fatalf("parse: %v", err)
	}

	doWork := store.GetNode("func:mypkg.DoWork")
	if doWork == nil {
		t.Fatal("missing DoWork function node")
	}
	if doWork.Kind != KindFunction {
		t.Fatalf("expected function kind, got %s", doWork.Kind)
	}
	if doWork.Metadata["exported"] != "true" {
		t.Fatal("DoWork should be marked as exported")
	}

	helperNode := store.GetNode("func:mypkg.helper")
	if helperNode == nil {
		t.Fatal("missing helper function node")
	}
	if helperNode.Metadata["exported"] != "false" {
		t.Fatal("helper should be marked as unexported")
	}
}

func TestParseSource_Method(t *testing.T) {
	p := NewCodeParser()
	store := NewGraphStore()
	src := `package mypkg

type Foo struct{}

func (f *Foo) Bar() {}
`
	if err := p.ParseSource(store, "mypkg.go", src); err != nil {
		t.Fatalf("parse: %v", err)
	}

	method := store.GetNode("method:mypkg.Foo.Bar")
	if method == nil {
		t.Fatal("missing method node")
	}
	if method.Kind != KindMethod {
		t.Fatalf("expected method kind, got %s", method.Kind)
	}

	// Receiver edge should exist.
	recvEdges := store.EdgesByKind(EdgeReceives)
	if len(recvEdges) != 1 {
		t.Fatalf("expected 1 receiver edge, got %d", len(recvEdges))
	}
	if recvEdges[0].From != "method:mypkg.Foo.Bar" {
		t.Fatalf("unexpected receiver edge from: %s", recvEdges[0].From)
	}
}

func TestParseSource_MethodValueReceiver(t *testing.T) {
	p := NewCodeParser()
	store := NewGraphStore()
	src := `package mypkg

type Foo struct{}

func (f Foo) Bar() {}
`
	if err := p.ParseSource(store, "mypkg.go", src); err != nil {
		t.Fatalf("parse: %v", err)
	}

	method := store.GetNode("method:mypkg.Foo.Bar")
	if method == nil {
		t.Fatal("missing method node for value receiver")
	}
}

func TestParseSource_Interface(t *testing.T) {
	p := NewCodeParser()
	store := NewGraphStore()
	src := `package mypkg

type Writer interface {
	Write([]byte) (int, error)
}
`
	if err := p.ParseSource(store, "mypkg.go", src); err != nil {
		t.Fatalf("parse: %v", err)
	}

	w := store.GetNode("type:mypkg.Writer")
	if w == nil {
		t.Fatal("missing Writer interface node")
	}
	if w.Kind != KindInterface {
		t.Fatalf("expected interface kind, got %s", w.Kind)
	}
}

func TestParseSource_Struct(t *testing.T) {
	p := NewCodeParser()
	store := NewGraphStore()
	src := `package mypkg

type Config struct {
	Name string
	Port int
}
`
	if err := p.ParseSource(store, "mypkg.go", src); err != nil {
		t.Fatalf("parse: %v", err)
	}

	cfg := store.GetNode("type:mypkg.Config")
	if cfg == nil {
		t.Fatal("missing Config type node")
	}
	if cfg.Kind != KindType {
		t.Fatalf("expected type kind, got %s", cfg.Kind)
	}

	nameField := store.GetNode("field:mypkg.Config.Name")
	if nameField == nil {
		t.Fatal("missing Name field node")
	}
	portField := store.GetNode("field:mypkg.Config.Port")
	if portField == nil {
		t.Fatal("missing Port field node")
	}
}

func TestParseSource_EmbeddedStruct(t *testing.T) {
	p := NewCodeParser()
	store := NewGraphStore()
	src := `package mypkg

type Base struct{}
type Derived struct {
	Base
}
`
	if err := p.ParseSource(store, "mypkg.go", src); err != nil {
		t.Fatalf("parse: %v", err)
	}

	embedEdges := store.EdgesByKind(EdgeEmbeds)
	if len(embedEdges) != 1 {
		t.Fatalf("expected 1 embed edge, got %d", len(embedEdges))
	}
	if embedEdges[0].From != "type:mypkg.Derived" {
		t.Fatalf("unexpected embed edge from: %s", embedEdges[0].From)
	}
}

func TestParseSource_Constants(t *testing.T) {
	p := NewCodeParser()
	store := NewGraphStore()
	src := `package mypkg

const (
	Foo = "foo"
	Bar = "bar"
)
`
	if err := p.ParseSource(store, "mypkg.go", src); err != nil {
		t.Fatalf("parse: %v", err)
	}

	foo := store.GetNode("constant:mypkg.Foo")
	if foo == nil {
		t.Fatal("missing constant Foo")
	}
	if foo.Kind != KindConstant {
		t.Fatalf("expected constant kind, got %s", foo.Kind)
	}
	bar := store.GetNode("constant:mypkg.Bar")
	if bar == nil {
		t.Fatal("missing constant Bar")
	}
}

func TestParseSource_Variables(t *testing.T) {
	p := NewCodeParser()
	store := NewGraphStore()
	src := `package mypkg

var (
	Debug bool
	_ = 42
)
`
	if err := p.ParseSource(store, "mypkg.go", src); err != nil {
		t.Fatalf("parse: %v", err)
	}

	debug := store.GetNode("variable:mypkg.Debug")
	if debug == nil {
		t.Fatal("missing variable Debug")
	}
	if debug.Kind != KindVariable {
		t.Fatalf("expected variable kind, got %s", debug.Kind)
	}

	// Blank identifier should be skipped.
	nodes := store.NodesByKind(KindVariable)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 variable (blank _ skipped), got %d", len(nodes))
	}
}

func TestParseSource_FunctionCalls(t *testing.T) {
	p := NewCodeParser()
	store := NewGraphStore()
	src := `package mypkg

import "fmt"

func Greet() {
	fmt.Println("hello")
}

func localCall() {
	Greet()
}
`
	if err := p.ParseSource(store, "mypkg.go", src); err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Check call from Greet to fmt.Println.
	greetEdges := store.OutEdges("func:mypkg.Greet")
	hasFmtCall := false
	for _, e := range greetEdges {
		if e.Kind == EdgeCalls && e.To == "ref:fmt.Println" {
			hasFmtCall = true
		}
	}
	if !hasFmtCall {
		t.Fatal("missing call edge from Greet to fmt.Println")
	}

	// Check call from localCall to Greet.
	localEdges := store.OutEdges("func:mypkg.localCall")
	hasGreetCall := false
	for _, e := range localEdges {
		if e.Kind == EdgeCalls && e.To == "func:mypkg.Greet" {
			hasGreetCall = true
		}
	}
	if !hasGreetCall {
		t.Fatal("missing call edge from localCall to Greet")
	}
}

func TestParseSource_DeclaredInEdges(t *testing.T) {
	p := NewCodeParser()
	store := NewGraphStore()
	src := `package mypkg

func Hello() {}
`
	if err := p.ParseSource(store, "mypkg.go", src); err != nil {
		t.Fatalf("parse: %v", err)
	}

	declEdges := store.EdgesByKind(EdgeDeclaredIn)
	if len(declEdges) == 0 {
		t.Fatal("expected declared_in edges")
	}

	// File should be declared in package.
	// Function should be declared in file.
	var fileInPkg, funcInFile bool
	for _, e := range declEdges {
		if strings.HasPrefix(e.From, "file:") && e.To == "pkg:mypkg" {
			fileInPkg = true
		}
		if e.From == "func:mypkg.Hello" && strings.HasPrefix(e.To, "file:") {
			funcInFile = true
		}
	}
	if !fileInPkg {
		t.Fatal("missing file->package declared_in edge")
	}
	if !funcInFile {
		t.Fatal("missing function->file declared_in edge")
	}
}

func TestParseSource_EmbeddedPointer(t *testing.T) {
	p := NewCodeParser()
	store := NewGraphStore()
	src := `package mypkg

type Base struct{}
type Derived struct {
	*Base
}
`
	if err := p.ParseSource(store, "mypkg.go", src); err != nil {
		t.Fatalf("parse: %v", err)
	}

	embedEdges := store.EdgesByKind(EdgeEmbeds)
	if len(embedEdges) != 1 {
		t.Fatalf("expected 1 embed edge for pointer embedding, got %d", len(embedEdges))
	}
}

func TestParseSource_MultipleFiles(t *testing.T) {
	p := NewCodeParser()
	store := NewGraphStore()

	src1 := `package mypkg
func Foo() {}
`
	src2 := `package mypkg
func Bar() {}
`
	if err := p.ParseSource(store, "a.go", src1); err != nil {
		t.Fatalf("parse a.go: %v", err)
	}
	if err := p.ParseSource(store, "b.go", src2); err != nil {
		t.Fatalf("parse b.go: %v", err)
	}

	if store.GetNode("func:mypkg.Foo") == nil {
		t.Fatal("missing Foo from first file")
	}
	if store.GetNode("func:mypkg.Bar") == nil {
		t.Fatal("missing Bar from second file")
	}
}

func TestReceiverTypeName_Table(t *testing.T) {
	// Tested indirectly through ParseSource with different receiver types.
	tests := []struct {
		name string
		src  string
		id   string
	}{
		{
			name: "pointer receiver",
			src:  "package p\ntype T struct{}\nfunc (t *T) M() {}",
			id:   "method:p.T.M",
		},
		{
			name: "value receiver",
			src:  "package p\ntype T struct{}\nfunc (t T) M() {}",
			id:   "method:p.T.M",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewCodeParser()
			store := NewGraphStore()
			if err := p.ParseSource(store, "test.go", tt.src); err != nil {
				t.Fatalf("parse: %v", err)
			}
			if store.GetNode(tt.id) == nil {
				t.Fatalf("missing node %s", tt.id)
			}
		})
	}
}
