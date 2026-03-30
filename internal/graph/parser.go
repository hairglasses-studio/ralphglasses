package graph

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

// CodeParser extracts entities and relationships from Go source using go/ast.
type CodeParser struct {
	fset *token.FileSet
}

// NewCodeParser creates a parser with a fresh file set.
func NewCodeParser() *CodeParser {
	return &CodeParser{fset: token.NewFileSet()}
}

// ParseFile parses a single Go source file and adds entities/relationships to the store.
func (p *CodeParser) ParseFile(store *GraphStore, path string) error {
	f, err := parser.ParseFile(p.fset, path, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("graph: parse %s: %w", path, err)
	}
	p.extractFromFile(store, f, path)
	return nil
}

// ParseDir parses all Go files in a directory and adds them to the store.
func (p *CodeParser) ParseDir(store *GraphStore, dir string) error {
	pkgs, err := parser.ParseDir(p.fset, dir, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("graph: parse dir %s: %w", dir, err)
	}
	for _, pkg := range pkgs {
		for path, f := range pkg.Files {
			p.extractFromFile(store, f, path)
		}
	}
	return nil
}

// ParseSource parses Go source provided as a string (useful for testing).
func (p *CodeParser) ParseSource(store *GraphStore, filename, src string) error {
	f, err := parser.ParseFile(p.fset, filename, src, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("graph: parse source: %w", err)
	}
	p.extractFromFile(store, f, filename)
	return nil
}

func (p *CodeParser) extractFromFile(store *GraphStore, f *ast.File, path string) {
	pkgName := f.Name.Name
	absPath, _ := filepath.Abs(path)

	// Package node.
	pkgID := "pkg:" + pkgName
	store.AddNode(&Node{
		ID:   pkgID,
		Kind: KindPackage,
		Name: pkgName,
	})

	// File node.
	fileID := "file:" + absPath
	store.AddNode(&Node{
		ID:      fileID,
		Kind:    KindFile,
		Name:    filepath.Base(path),
		Package: pkgName,
		File:    absPath,
	})
	_ = store.AddEdge(&Edge{From: fileID, To: pkgID, Kind: EdgeDeclaredIn})

	// Imports.
	for _, imp := range f.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		impID := "pkg:" + importPath
		store.AddNode(&Node{
			ID:   impID,
			Kind: KindPackage,
			Name: filepath.Base(importPath),
		})
		_ = store.AddEdge(&Edge{From: pkgID, To: impID, Kind: EdgeImports})
	}

	// Walk declarations.
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			p.extractFunc(store, d, pkgName, pkgID, fileID)
		case *ast.GenDecl:
			p.extractGenDecl(store, d, pkgName, pkgID, fileID)
		}
	}
}

func (p *CodeParser) extractFunc(store *GraphStore, fn *ast.FuncDecl, pkg, pkgID, fileID string) {
	pos := p.fset.Position(fn.Pos())

	var funcID string
	var recvTypeID string
	kind := KindFunction
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		kind = KindMethod
		recvType := receiverTypeName(fn.Recv.List[0].Type)
		funcID = fmt.Sprintf("method:%s.%s.%s", pkg, recvType, fn.Name.Name)
		recvTypeID = "type:" + pkg + "." + recvType
		store.AddNode(&Node{ID: recvTypeID, Kind: KindType, Name: recvType, Package: pkg})
	} else {
		funcID = fmt.Sprintf("func:%s.%s", pkg, fn.Name.Name)
	}

	// Add the function/method node before creating edges from it.
	store.AddNode(&Node{
		ID:      funcID,
		Kind:    kind,
		Name:    fn.Name.Name,
		Package: pkg,
		File:    pos.Filename,
		Line:    pos.Line,
		Metadata: map[string]string{
			"exported": fmt.Sprintf("%t", fn.Name.IsExported()),
		},
	})
	_ = store.AddEdge(&Edge{From: funcID, To: fileID, Kind: EdgeDeclaredIn})

	// Edge from method to receiver type (after method node exists).
	if recvTypeID != "" {
		_ = store.AddEdge(&Edge{From: funcID, To: recvTypeID, Kind: EdgeReceives})
	}

	// Extract function calls from the body.
	if fn.Body != nil {
		p.extractCalls(store, fn.Body, funcID, pkg)
	}
}

func (p *CodeParser) extractCalls(store *GraphStore, body *ast.BlockStmt, callerID, pkg string) {
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		var calleeID string
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			// Local function call.
			calleeID = "func:" + pkg + "." + fun.Name
		case *ast.SelectorExpr:
			if ident, ok := fun.X.(*ast.Ident); ok {
				// Could be pkg.Func or receiver.Method — we use references for both.
				calleeID = "ref:" + ident.Name + "." + fun.Sel.Name
			}
		}

		if calleeID != "" {
			// Ensure a placeholder node exists for the callee.
			if store.GetNode(calleeID) == nil {
				store.AddNode(&Node{
					ID:   calleeID,
					Kind: KindFunction,
					Name: calleeID,
				})
			}
			_ = store.AddEdge(&Edge{From: callerID, To: calleeID, Kind: EdgeCalls})
		}
		return true
	})
}

func (p *CodeParser) extractGenDecl(store *GraphStore, gd *ast.GenDecl, pkg, pkgID, fileID string) {
	for _, spec := range gd.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			p.extractTypeSpec(store, s, pkg, pkgID, fileID)
		case *ast.ValueSpec:
			p.extractValueSpec(store, s, gd.Tok, pkg, fileID)
		}
	}
}

func (p *CodeParser) extractTypeSpec(store *GraphStore, ts *ast.TypeSpec, pkg, pkgID, fileID string) {
	pos := p.fset.Position(ts.Pos())
	kind := KindType

	if _, ok := ts.Type.(*ast.InterfaceType); ok {
		kind = KindInterface
	}

	typeID := fmt.Sprintf("type:%s.%s", pkg, ts.Name.Name)
	store.AddNode(&Node{
		ID:      typeID,
		Kind:    kind,
		Name:    ts.Name.Name,
		Package: pkg,
		File:    pos.Filename,
		Line:    pos.Line,
		Metadata: map[string]string{
			"exported": fmt.Sprintf("%t", ts.Name.IsExported()),
		},
	})
	_ = store.AddEdge(&Edge{From: typeID, To: fileID, Kind: EdgeDeclaredIn})

	// Struct fields and embedded types.
	if st, ok := ts.Type.(*ast.StructType); ok {
		for _, field := range st.Fields.List {
			if len(field.Names) == 0 {
				// Embedded type.
				embeddedName := typeExprName(field.Type)
				if embeddedName != "" {
					embID := "type:" + pkg + "." + embeddedName
					store.AddNode(&Node{ID: embID, Kind: KindType, Name: embeddedName, Package: pkg})
					_ = store.AddEdge(&Edge{From: typeID, To: embID, Kind: EdgeEmbeds})
				}
			} else {
				for _, name := range field.Names {
					fieldID := fmt.Sprintf("field:%s.%s.%s", pkg, ts.Name.Name, name.Name)
					store.AddNode(&Node{
						ID:      fieldID,
						Kind:    KindField,
						Name:    name.Name,
						Package: pkg,
						File:    pos.Filename,
						Line:    p.fset.Position(name.Pos()).Line,
					})
					_ = store.AddEdge(&Edge{From: fieldID, To: typeID, Kind: EdgeDeclaredIn})
				}
			}
		}
	}

	// Interface methods — record implements edges will require cross-referencing
	// which is done at query time, not parse time.
}

func (p *CodeParser) extractValueSpec(store *GraphStore, vs *ast.ValueSpec, tok token.Token, pkg, fileID string) {
	kind := KindVariable
	if tok == token.CONST {
		kind = KindConstant
	}
	for _, name := range vs.Names {
		if name.Name == "_" {
			continue
		}
		pos := p.fset.Position(name.Pos())
		id := fmt.Sprintf("%s:%s.%s", kind, pkg, name.Name)
		store.AddNode(&Node{
			ID:      id,
			Kind:    kind,
			Name:    name.Name,
			Package: pkg,
			File:    pos.Filename,
			Line:    pos.Line,
		})
		_ = store.AddEdge(&Edge{From: id, To: fileID, Kind: EdgeDeclaredIn})
	}
}

// receiverTypeName extracts the type name from a method receiver expression.
func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.IndexExpr: // generic receiver T[X]
		return receiverTypeName(t.X)
	case *ast.IndexListExpr: // generic receiver T[X, Y]
		return receiverTypeName(t.X)
	}
	return ""
}

// typeExprName extracts a simple type name from an expression.
func typeExprName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return typeExprName(t.X)
	case *ast.SelectorExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name + "." + t.Sel.Name
		}
	}
	return ""
}
