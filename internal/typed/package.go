// Package typed provides type-checked analysis of Go packages using go/types
// and golang.org/x/tools/go/packages.
//
// While the sibling `analyzer` package works purely on the syntactic level
// (go/ast + name-based heuristics), `typed` consumes full type information
// from the Go type checker. This enables:
//
//   - Precise interface satisfaction: types.Implements instead of method-name
//     comparison.
//   - Accurate call graph: types.Info.Uses resolves an identifier to the
//     exact *types.Func it refers to, rather than a best-effort name match
//     across all functions in a package.
//   - Resolution of symbols imported from other packages, including aliases,
//     generics (type arguments), and embedded fields.
//
// The package is intentionally small and focused on the two places where
// syntactic analysis is most visibly inadequate in the pipeline (interface
// implementors and callees). Everything else continues to be handled by the
// `analyzer` package, which is perfectly adequate for extracting signatures
// and bodies for prompt construction.
package typed

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// Package wraps a type-checked Go package.
//
// All fields are populated by Load. A nil receiver is not valid.
type Package struct {
	// Dir is the directory on disk that was loaded.
	Dir string
	// Path is the import path reported by go/packages. May be empty for
	// ad-hoc packages or when the directory is outside a module.
	Path string
	// Fset is the FileSet shared by all parsed files and the type checker.
	Fset *token.FileSet
	// Types is the type-checked package.
	Types *types.Package
	// Info holds type information for expressions and declarations.
	Info *types.Info
	// Syntax is the list of parsed file ASTs that back Types.
	Syntax []*ast.File
}

// LoadResult records the outcome of Load, including any non-fatal diagnostics.
type LoadResult struct {
	Package  *Package
	Warnings []string
}

// Load type-checks the Go package located in dir and returns the result.
//
// Non-fatal errors reported by the loader (for example, import resolution
// problems in a partially vendored project) are returned as Warnings; the
// caller can still use the Package if it was successfully type-checked.
// A hard error is returned only when type information could not be produced
// at all.
func Load(dir string) (*LoadResult, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedTypes |
			packages.NeedSyntax | packages.NeedTypesInfo,
		Dir:   dir,
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, fmt.Errorf("load package: %w", err)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no Go packages found in %s", dir)
	}
	p := pkgs[0]

	var warnings []string
	for _, e := range p.Errors {
		warnings = append(warnings, e.Error())
	}

	if p.Types == nil || p.TypesInfo == nil {
		return &LoadResult{Warnings: warnings}, errors.New("type information unavailable")
	}

	return &LoadResult{
		Package: &Package{
			Dir:    dir,
			Path:   p.PkgPath,
			Fset:   p.Fset,
			Types:  p.Types,
			Info:   p.TypesInfo,
			Syntax: p.Syntax,
		},
		Warnings: warnings,
	}, nil
}

// Implementors returns the names of concrete types declared in this package
// that implement the named interface.
//
// Both value and pointer receiver sets are checked, mirroring the semantics
// of `interface satisfaction` in the Go language specification.
// Interface types themselves are excluded from the result.
// The interface is looked up in the package scope by bare name.
func (p *Package) Implementors(ifaceName string) []string {
	if p == nil || p.Types == nil {
		return nil
	}
	obj := p.Types.Scope().Lookup(ifaceName)
	if obj == nil {
		return nil
	}
	iface, ok := obj.Type().Underlying().(*types.Interface)
	if !ok {
		return nil
	}

	var out []string
	scope := p.Types.Scope()
	for _, name := range scope.Names() {
		o := scope.Lookup(name)
		tn, ok := o.(*types.TypeName)
		if !ok || tn.Name() == ifaceName {
			continue
		}
		T := tn.Type()
		// Skip interface types themselves.
		if _, isIface := T.Underlying().(*types.Interface); isIface {
			continue
		}
		if types.Implements(T, iface) || types.Implements(types.NewPointer(T), iface) {
			out = append(out, tn.Name())
		}
	}
	return out
}

// Callee describes a single call resolved by the type checker.
type Callee struct {
	// Name is the unqualified name of the called function or method.
	Name string
	// Receiver holds the receiver type name for method calls ("" for free
	// functions). It is reported without a leading '*'.
	Receiver string
	// SamePackage indicates whether the callee is defined in this package.
	SamePackage bool
}

// Callees returns the distinct same-package callees of the named function.
//
// The function itself is looked up by unqualified name among the package's
// top-level declarations (methods are ignored; callers that need methods
// should use CalleesOfMethod). Each call expression within the function
// body is resolved via types.Info.Uses to locate the *types.Func being
// called. Identifiers that do not resolve to a function (variables of
// function type, closures, etc.) are skipped.
//
// Recursive calls to the function itself are omitted.
func (p *Package) Callees(fnName string) []Callee {
	fnDecl := p.findTopLevelFunc(fnName)
	if fnDecl == nil || fnDecl.Body == nil {
		return nil
	}
	return p.calleesInBody(fnDecl.Body, fnName)
}

// CalleesOfMethod is the method-aware counterpart of Callees. It looks up a
// method by its receiver type and method name, then returns the callees of
// that method's body.
func (p *Package) CalleesOfMethod(receiverType, methodName string) []Callee {
	decl := p.findMethod(receiverType, methodName)
	if decl == nil || decl.Body == nil {
		return nil
	}
	return p.calleesInBody(decl.Body, methodName)
}

func (p *Package) calleesInBody(body *ast.BlockStmt, selfName string) []Callee {
	seen := map[string]struct{}{}
	var out []Callee

	ast.Inspect(body, func(n ast.Node) bool {
		ce, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		var ident *ast.Ident
		switch f := ce.Fun.(type) {
		case *ast.Ident:
			ident = f
		case *ast.SelectorExpr:
			ident = f.Sel
		case *ast.IndexExpr:
			if id, ok := f.X.(*ast.Ident); ok {
				ident = id
			}
		case *ast.IndexListExpr:
			if id, ok := f.X.(*ast.Ident); ok {
				ident = id
			}
		}
		if ident == nil {
			return true
		}

		obj := p.Info.Uses[ident]
		if obj == nil {
			return true
		}
		fn, ok := obj.(*types.Func)
		if !ok {
			return true
		}

		sig, ok := fn.Type().(*types.Signature)
		if !ok {
			return true
		}

		samePkg := fn.Pkg() == p.Types

		var receiver string
		if recv := sig.Recv(); recv != nil {
			receiver = typeNameOf(recv.Type())
		}

		key := receiver + "." + fn.Name()
		if _, dup := seen[key]; dup {
			return true
		}
		seen[key] = struct{}{}

		// Skip direct self-recursion.
		if fn.Name() == selfName && receiver == "" {
			return true
		}

		out = append(out, Callee{
			Name:        fn.Name(),
			Receiver:    receiver,
			SamePackage: samePkg,
		})
		return true
	})

	return out
}

// typeNameOf returns the unqualified name of a named type, stripping a
// leading '*' for pointer receivers. Returns empty for types without a name
// (anonymous, composite, etc.).
func typeNameOf(t types.Type) string {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	if named, ok := t.(*types.Named); ok {
		return named.Obj().Name()
	}
	return ""
}

func (p *Package) findTopLevelFunc(name string) *ast.FuncDecl {
	for _, file := range p.Syntax {
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv != nil {
				continue
			}
			if fd.Name.Name == name {
				return fd
			}
		}
	}
	return nil
}

func (p *Package) findMethod(receiverType, methodName string) *ast.FuncDecl {
	for _, file := range p.Syntax {
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || fd.Name.Name != methodName {
				continue
			}
			if rn := recvTypeName(fd.Recv.List[0].Type); rn == receiverType {
				return fd
			}
		}
	}
	return nil
}

func recvTypeName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.StarExpr:
		return recvTypeName(x.X)
	case *ast.Ident:
		return x.Name
	case *ast.IndexExpr:
		return recvTypeName(x.X)
	case *ast.IndexListExpr:
		return recvTypeName(x.X)
	}
	return ""
}
