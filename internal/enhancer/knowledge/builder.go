package knowledge

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// Builder constructs a knowledge graph from Go source code.
type Builder struct {
	graph *Graph
	fset  *token.FileSet
}

// NewBuilder creates a builder that populates the given graph.
func NewBuilder(g *Graph) *Builder {
	return &Builder{
		graph: g,
		fset:  token.NewFileSet(),
	}
}

// BuildFromDir scans a Go directory tree, parsing source files to build the graph.
// It extracts package structure, type definitions, function signatures, and imports.
// Only signatures are stored — no full function bodies.
func (b *Builder) BuildFromDir(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		// Skip hidden directories and vendor/testdata
		base := info.Name()
		if info.IsDir() {
			if strings.HasPrefix(base, ".") || base == "vendor" || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		// Only process .go files (skip test files for graph building)
		if !strings.HasSuffix(base, ".go") || strings.HasSuffix(base, "_test.go") {
			return nil
		}
		return b.parseFile(path, dir)
	})
}

// parseFile parses a single Go file and adds its entities to the graph.
func (b *Builder) parseFile(path, rootDir string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil // skip unreadable files
	}

	f, err := parser.ParseFile(b.fset, path, src, parser.ParseComments)
	if err != nil {
		return nil // skip unparseable files
	}

	relPath, _ := filepath.Rel(rootDir, path)
	if relPath == "" {
		relPath = path
	}

	pkgName := f.Name.Name
	pkgID := "pkg:" + pkgName
	fileID := "file:" + relPath

	// Add package node
	b.graph.AddNode(pkgID, KindPackage, map[string]string{
		"name": pkgName,
	})

	// Add file node
	b.graph.AddNode(fileID, KindFile, map[string]string{
		"path":    relPath,
		"package": pkgName,
	})

	// File belongs to package
	b.graph.AddEdge(pkgID, fileID, EdgeDefines)

	// Process imports
	for _, imp := range f.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		importID := "pkg:" + importPath
		b.graph.AddNode(importID, KindPackage, map[string]string{
			"name":     filepath.Base(importPath),
			"path":     importPath,
			"external": "true",
		})
		b.graph.AddEdge(pkgID, importID, EdgeImports)
	}

	// Process declarations
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			b.processGenDecl(d, fileID, pkgID)
		case *ast.FuncDecl:
			b.processFuncDecl(d, fileID, pkgID, pkgName)
		}
	}

	return nil
}

// processGenDecl handles type and const/var declarations.
func (b *Builder) processGenDecl(decl *ast.GenDecl, fileID, pkgID string) {
	if decl.Tok != token.TYPE {
		return
	}

	for _, spec := range decl.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}

		typeID := pkgID + "." + ts.Name.Name
		md := map[string]string{
			"name": ts.Name.Name,
		}

		switch t := ts.Type.(type) {
		case *ast.StructType:
			md["kind"] = "struct"
			if t.Fields != nil {
				var fields []string
				for _, f := range t.Fields.List {
					for _, name := range f.Names {
						fields = append(fields, name.Name)
					}
				}
				if len(fields) > 0 {
					md["fields"] = strings.Join(fields, ", ")
				}
			}
		case *ast.InterfaceType:
			md["kind"] = "interface"
			if t.Methods != nil {
				var methods []string
				for _, m := range t.Methods.List {
					for _, name := range m.Names {
						methods = append(methods, name.Name)
					}
				}
				if len(methods) > 0 {
					md["methods"] = strings.Join(methods, ", ")
				}
			}
		default:
			md["kind"] = "alias"
		}

		b.graph.AddNode(typeID, KindType, md)
		b.graph.AddEdge(fileID, typeID, EdgeDefines)

		// Check for interface implementations (embedded interfaces)
		if iface, ok := ts.Type.(*ast.InterfaceType); ok && iface.Methods != nil {
			for _, m := range iface.Methods.List {
				if len(m.Names) == 0 {
					// Embedded interface
					if ident, ok := m.Type.(*ast.Ident); ok {
						embeddedID := pkgID + "." + ident.Name
						b.graph.AddEdge(typeID, embeddedID, EdgeImplements)
					}
				}
			}
		}
	}
}

// processFuncDecl handles function and method declarations.
func (b *Builder) processFuncDecl(decl *ast.FuncDecl, fileID, pkgID, pkgName string) {
	funcName := decl.Name.Name
	var funcID string
	md := map[string]string{
		"name": funcName,
	}

	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		// Method
		recvType := receiverTypeName(decl.Recv.List[0].Type)
		funcID = pkgID + "." + recvType + "." + funcName
		md["receiver"] = recvType
		md["kind"] = "method"
	} else {
		funcID = pkgID + "." + funcName
		md["kind"] = "function"
	}

	// Extract signature
	md["signature"] = formatFuncSignature(decl, pkgName)

	// exported?
	if ast.IsExported(funcName) {
		md["exported"] = "true"
	}

	b.graph.AddNode(funcID, KindFunction, md)
	b.graph.AddEdge(fileID, funcID, EdgeDefines)

	// If it's a method, link to the receiver type
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		recvType := receiverTypeName(decl.Recv.List[0].Type)
		typeID := pkgID + "." + recvType
		b.graph.AddEdge(typeID, funcID, EdgeDefines)
	}

	// Scan body for function calls to build call edges
	if decl.Body != nil {
		b.extractCalls(decl.Body, funcID, pkgID)
	}
}

// extractCalls walks a function body looking for call expressions.
func (b *Builder) extractCalls(body *ast.BlockStmt, callerID, pkgID string) {
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		switch fn := call.Fun.(type) {
		case *ast.Ident:
			// Local function call
			calleeID := pkgID + "." + fn.Name
			b.graph.AddEdge(callerID, calleeID, EdgeCalls)
		case *ast.SelectorExpr:
			// pkg.Func or receiver.Method
			if ident, ok := fn.X.(*ast.Ident); ok {
				calleeID := pkgID + "." + ident.Name + "." + fn.Sel.Name
				b.graph.AddEdge(callerID, calleeID, EdgeCalls)
			}
		}
		return true
	})
}

// receiverTypeName extracts the type name from a receiver expression,
// stripping pointer indirection.
func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.Ident:
		return t.Name
	default:
		return "unknown"
	}
}

// formatFuncSignature formats a function declaration as a signature string.
func formatFuncSignature(decl *ast.FuncDecl, pkgName string) string {
	var b strings.Builder
	b.WriteString("func ")

	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		recvType := receiverTypeName(decl.Recv.List[0].Type)
		// Check if pointer receiver
		if _, ok := decl.Recv.List[0].Type.(*ast.StarExpr); ok {
			fmt.Fprintf(&b, "(%s *%s) ", decl.Recv.List[0].Names[0].Name, recvType)
		} else if len(decl.Recv.List[0].Names) > 0 {
			fmt.Fprintf(&b, "(%s %s) ", decl.Recv.List[0].Names[0].Name, recvType)
		}
	}

	b.WriteString(decl.Name.Name)
	b.WriteString("(")

	if decl.Type.Params != nil {
		var params []string
		for _, p := range decl.Type.Params.List {
			typeName := exprString(p.Type)
			if len(p.Names) == 0 {
				params = append(params, typeName)
			} else {
				for _, name := range p.Names {
					params = append(params, name.Name+" "+typeName)
				}
			}
		}
		b.WriteString(strings.Join(params, ", "))
	}
	b.WriteString(")")

	if decl.Type.Results != nil && len(decl.Type.Results.List) > 0 {
		var results []string
		for _, r := range decl.Type.Results.List {
			typeName := exprString(r.Type)
			if len(r.Names) > 0 {
				for _, name := range r.Names {
					results = append(results, name.Name+" "+typeName)
				}
			} else {
				results = append(results, typeName)
			}
		}
		if len(results) == 1 {
			b.WriteString(" " + results[0])
		} else {
			b.WriteString(" (" + strings.Join(results, ", ") + ")")
		}
	}

	return b.String()
}

// exprString returns a simplified string representation of a type expression.
func exprString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprString(t.X)
	case *ast.SelectorExpr:
		return exprString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + exprString(t.Elt)
		}
		return "[...]" + exprString(t.Elt)
	case *ast.MapType:
		return "map[" + exprString(t.Key) + "]" + exprString(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func(...)"
	case *ast.Ellipsis:
		return "..." + exprString(t.Elt)
	case *ast.ChanType:
		return "chan " + exprString(t.Value)
	default:
		return "any"
	}
}
