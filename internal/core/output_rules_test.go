package core

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestNoForbiddenStdOutputCallsInProductionCode(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve test file path")
	}

	// internal/core -> repo root
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	fset := token.NewFileSet()
	var violations []string

	walkErr := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".serena" || name == ".zcache" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			switch fn := call.Fun.(type) {
			case *ast.Ident:
				if fn.Name == "println" {
					pos := fset.Position(fn.Pos())
					violations = append(violations, pos.String()+" uses println")
				}
			case *ast.SelectorExpr:
				pkg, pkgOK := fn.X.(*ast.Ident)
				if !pkgOK {
					return true
				}

				if pkg.Name == "fmt" && fn.Sel.Name == "Println" {
					pos := fset.Position(fn.Pos())
					violations = append(violations, pos.String()+" uses fmt.Println")
				}

				if pkg.Name == "log" && (fn.Sel.Name == "Printf" || fn.Sel.Name == "Fatalf") {
					pos := fset.Position(fn.Pos())
					violations = append(violations, pos.String()+" uses log."+fn.Sel.Name)
				}
			}

			return true
		})

		return nil
	})

	if walkErr != nil {
		t.Fatalf("failed to scan repository: %v", walkErr)
	}

	if len(violations) > 0 {
		slices.Sort(violations)
		t.Fatalf("found forbidden output calls in production code:\n%s", strings.Join(violations, "\n"))
	}
}
