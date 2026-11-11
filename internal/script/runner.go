package script

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func Resolve(dir, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("empty script name")
	}
	name = filepath.Base(name)
	name = strings.TrimSuffix(name, ".py")
	name = strings.TrimSuffix(name, ".sh")
	candidates := []string{
		filepath.Join(dir, name+".sh"),
		filepath.Join(dir, name+".py"),
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("script not found: %s", name)
}

func Run(dir, name string, params []string, timeout time.Duration) (string, string, error) {
	path, err := Resolve(dir, name)
	if err != nil {
		return "", "", err
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var cmd *exec.Cmd
	switch strings.ToLower(filepath.Ext(path)) {
	case ".py":
		cmd = exec.CommandContext(ctx, "python3", append([]string{filepath.Base(path)}, params...)...)
	default:
		cmd = exec.CommandContext(ctx, "./"+filepath.Base(path), params...)
	}
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	out := strings.TrimSpace(stdout.String())
	errOut := strings.TrimSpace(stderr.String())
	if ctx.Err() == context.DeadlineExceeded {
		return out, errOut, fmt.Errorf("script timeout")
	}
	if err != nil {
		if errOut != "" {
			return out, errOut, fmt.Errorf("%w: %s", err, errOut)
		}
		return out, errOut, err
	}
	return out, errOut, nil
}
