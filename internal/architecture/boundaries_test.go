package architecture_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDependencyBoundaries(t *testing.T) {
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		path = filepath.ToSlash(path)
		for _, imp := range file.Imports {
			value, _ := strconv.Unquote(imp.Path.Value)
			isNonStandard := strings.Contains(value, ".") || strings.HasPrefix(value, "boilerplate/")
			isDomain := strings.Contains(path, "/domain/")
			isApplication := strings.Contains(path, "/application/")
			if isDomain && isNonStandard {
				t.Errorf("domain %s imports %s", path, value)
			}
			if isApplication &&
				isNonStandard &&
				!strings.HasPrefix(value, "boilerplate/internal/users/domain") {
				t.Errorf("application %s imports %s", path, value)
			}
			if !strings.Contains(path, "/app/") && strings.Contains(value, "boilerplate/internal/app") {
				t.Errorf("lower layer imports composition root: %s", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
