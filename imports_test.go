package markitdown_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestImportEdges(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", "{{.ImportPath}}\t{{join .Imports \",\"}}", "./...").CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	root := "github.com/godwit-email/markitdown-go"
	formats := []string{
		root + "/csv", root + "/docx", root + "/html", root + "/pdf", root + "/pptx",
		root + "/text", root + "/xls", root + "/xlsx", root + "/zip",
	}
	has := func(imports []string, want string) bool {
		for _, im := range imports {
			if im == want {
				return true
			}
		}
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pkg, rest, _ := strings.Cut(line, "\t")
		var imports []string
		if rest != "" {
			imports = strings.Split(rest, ",")
		}
		if pkg == root {
			for _, f := range formats {
				if has(imports, f) {
					t.Errorf("root imports %s", f)
				}
			}
		}
		if strings.HasPrefix(pkg, root+"/internal/") {
			if has(imports, root) {
				t.Errorf("%s imports root", pkg)
			}
			for _, f := range formats {
				if has(imports, f) {
					t.Errorf("%s imports %s", pkg, f)
				}
			}
		}
		for _, f := range formats {
			if pkg != f {
				continue
			}
			for _, other := range formats {
				if other != f && has(imports, other) {
					t.Errorf("%s imports %s", f, other)
				}
			}
		}
	}
}
