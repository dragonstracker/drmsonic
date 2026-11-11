package ui

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

var scriptAction = regexp.MustCompile(`\{\{[^}]*\}\}`)

func TestScriptActionsAreQuoted(t *testing.T) {
	err := fs.WalkDir(Templates(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		raw, err := fs.ReadFile(Templates(), path)
		if err != nil {
			return err
		}
		inScript := false
		for i, line := range strings.Split(string(raw), "\n") {
			if strings.Contains(line, "<script") {
				inScript = true
			}
			if inScript {
				for _, m := range scriptAction.FindAllStringIndex(line, -1) {
					if insideJSString(line, m[0]) {
						continue
					}
					before := byte(' ')
					if m[0] > 0 {
						before = line[m[0]-1]
					}
					after := byte(' ')
					if m[1] < len(line) {
						after = line[m[1]]
					}
					if isIdentByte(before) || after == '.' || after == '(' || after == '[' {
						t.Errorf("%s:%d: template action in script must be quoted: %s", path, i+1, strings.TrimSpace(line))
					}
				}
			}
			if strings.Contains(line, "</script>") {
				inScript = false
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func insideJSString(line string, pos int) bool {
	single, double := false, false
	for i := 0; i < pos && i < len(line); i++ {
		switch line[i] {
		case '\'':
			if !double {
				single = !single
			}
		case '"':
			if !single {
				double = !double
			}
		}
	}
	return single || double
}

func isIdentByte(b byte) bool {
	return b == '_' || b == '$' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}
