package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var matrixEntry = regexp.MustCompile(`pkg: (\S+), name: (Fuzz\w+)`)

func TestFuzzWorkflowListsEveryTarget(t *testing.T) {
	workflow, err := os.ReadFile(filepath.Join(".github", "workflows", "fuzz.yml"))
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, m := range matrixEntry.FindAllStringSubmatch(string(workflow), -1) {
		listed[filepath.Clean(m[1])+"."+m[2]] = true
	}
	defined := map[string]bool{}
	fset := token.NewFileSet()
	err = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Fuzz") {
				defined[filepath.Dir(path)+"."+fn.Name.Name] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(defined) == 0 {
		t.Fatal("no Fuzz function found under the module root")
	}
	want, got := slices.Sorted(maps.Keys(defined)), slices.Sorted(maps.Keys(listed))
	if !slices.Equal(want, got) {
		t.Fatalf("fuzz.yml matrix lists %v; the code defines %v", got, want)
	}
}
