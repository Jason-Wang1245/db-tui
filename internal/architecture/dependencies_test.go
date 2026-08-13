package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/Jason-Wang1245/db-tui/"

func TestPackageDependencyRules(t *testing.T) {
	root := repositoryRoot(t)
	features := map[string]bool{
		"grid": true, "launcher": true, "profile": true, "sqltab": true, "workspace": true,
	}

	for _, packageName := range []string{"app", "core", "grid", "launcher", "platform", "postgres", "profile", "sqltab", "ui", "workspace"} {
		imports := productionImports(t, filepath.Join(root, "internal", packageName))
		for _, imported := range imports {
			if packageName == "app" && (internalPackage(imported) == "platform" || internalPackage(imported) == "postgres") {
				t.Errorf("internal/app must not import concrete adapter %q", imported)
			}
			if features[packageName] && features[internalPackage(imported)] && internalPackage(imported) != packageName {
				t.Errorf("feature internal/%s must not import feature %q", packageName, imported)
			}
			if (packageName == "platform" || packageName == "postgres") && strings.HasPrefix(imported, "charm.land/") {
				t.Errorf("adapter internal/%s must not import UI dependency %q", packageName, imported)
			}
			if packageName == "core" && strings.HasPrefix(imported, modulePath+"internal/") {
				t.Errorf("internal/core must not import internal package %q", imported)
			}
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("could not locate go.mod")
		}
		directory = parent
	}
}

func productionImports(t *testing.T, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var imports []string
	files := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(files, filepath.Join(directory, entry.Name()), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Decls {
			declaration, ok := spec.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, item := range declaration.Specs {
				importSpec, ok := item.(*ast.ImportSpec)
				if !ok {
					continue
				}
				path, err := strconv.Unquote(importSpec.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				imports = append(imports, path)
			}
		}
	}
	return imports
}

func internalPackage(importPath string) string {
	prefix := modulePath + "internal/"
	if !strings.HasPrefix(importPath, prefix) {
		return ""
	}
	remainder := strings.TrimPrefix(importPath, prefix)
	return strings.SplitN(remainder, "/", 2)[0]
}
