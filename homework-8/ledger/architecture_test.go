package ledger

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDependencyDirection(t *testing.T) {
	for _, root := range []string{"internal/domain", "internal/service", "internal/repository/pg", "../gateway"} {
		t.Run(root, func(t *testing.T) {
			err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
					return nil
				}
				file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
				if err != nil {
					return err
				}
				for _, spec := range file.Imports {
					imp, _ := strconv.Unquote(spec.Path.Value)
					bad := false
					switch root {
					case "internal/domain":
						bad = strings.HasPrefix(imp, "homework-8/") || strings.HasPrefix(imp, "github.com/") || imp == "database/sql" || imp == "net/http"
					case "internal/service":
						bad = strings.HasPrefix(imp, "homework-8/") && imp != "homework-8/ledger/internal/domain" || strings.HasPrefix(imp, "github.com/") || imp == "database/sql" || imp == "net/http"
					case "internal/repository/pg":
						bad = strings.Contains(imp, "gateway") || strings.Contains(imp, "/service") || imp == "net/http"
					case "../gateway":
						bad = imp == "database/sql" || strings.Contains(imp, "pgx") || strings.Contains(imp, "go-redis") || strings.Contains(imp, "/ledger/")
					}
					if bad {
						t.Errorf("%s imports forbidden dependency %s", path, imp)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
