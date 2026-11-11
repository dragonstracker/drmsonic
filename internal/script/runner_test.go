package script

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestResolvePrefersShThenPy(t *testing.T) {
	dir := t.TempDir()
	py := filepath.Join(dir, "demo.py")
	if err := os.WriteFile(py, []byte("print('py')\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(dir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if got != py {
		t.Fatalf("got %s want %s", got, py)
	}
	if runtime.GOOS == "windows" {
		return
	}
	sh := filepath.Join(dir, "demo.sh")
	if err := os.WriteFile(sh, []byte("#!/bin/sh\necho sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err = Resolve(dir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if got != sh {
		t.Fatalf("got %s want %s", got, sh)
	}
}

func TestRunPython(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	dir := t.TempDir()
	py := filepath.Join(dir, "echo.py")
	src := "import sys\nprint('ok=' + sys.argv[1])\n"
	if err := os.WriteFile(py, []byte(src), 0o755); err != nil {
		t.Fatal(err)
	}
	out, _, err := Run(dir, "echo", []string{"action=events"}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok=action=events" {
		t.Fatalf("stdout %q", out)
	}
}
