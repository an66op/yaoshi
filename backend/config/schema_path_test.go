package config

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestApplicationSchemaChangesStayInVersionedMigrations(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate backend sources")
	}
	root := filepath.Dir(filepath.Dir(filename))
	ddl := regexp.MustCompile(`(?is)\b(?:CREATE(?:\s+OR\s+REPLACE)?(?:\s+UNIQUE)?|ALTER|DROP)\s+(?:TABLE|INDEX|SEQUENCE|FUNCTION|TRIGGER|SCHEMA|VIEW)\b`)
	files := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		parsed, err := parser.ParseFile(files, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			if call, ok := node.(*ast.CallExpr); ok {
				if selector, ok := call.Fun.(*ast.SelectorExpr); ok && (selector.Sel.Name == "AutoMigrate" || selector.Sel.Name == "Migrator") {
					t.Errorf("model-derived schema mutation is forbidden: %s", files.Position(call.Pos()))
				}
			}
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err != nil || !ddl.MatchString(value) {
				return true
			}
			// The runner owns only its metadata table. Application tables,
			// indexes and trigger installation must be checksummed SQL files.
			if filepath.ToSlash(relative) == "migrations/runner.go" && strings.Contains(value, "CREATE TABLE IF NOT EXISTS schema_migrations") {
				return true
			}
			// A regexp that describes the SQL inventory is not an executed DDL.
			if filepath.ToSlash(relative) == "migrations/runner.go" && strings.Contains(value, "(?ms)") {
				return true
			}
			t.Errorf("unversioned schema SQL in application source: %s", files.Position(literal.Pos()))
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal("scan schema mutation paths:", err)
	}
}
