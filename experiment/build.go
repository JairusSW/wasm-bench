package experiment

import (
	"context"
	"fmt"
	"github.com/wasmbench/wasmbench/corpus"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// BuildWago resolves a local source checkout without writing into that checkout.
// The executable records its commit and a digest of all tracked Go/assembly inputs.
func BuildWago(ctx context.Context, root, source string) error {
	source, e := filepath.Abs(source)
	if e != nil {
		return e
	}
	cmd := exec.CommandContext(ctx, "git", "-C", source, "rev-parse", "HEAD")
	revision, e := cmd.Output()
	if e != nil {
		return fmt.Errorf("Wago source checkout: %w", e)
	}
	cmd = exec.CommandContext(ctx, "git", "-C", source, "ls-files", "-z", "*.go", "*.s", "*.S", "go.mod")
	files, e := cmd.Output()
	if e != nil {
		return e
	}
	var inputs strings.Builder
	for _, rel := range strings.Split(string(files), "\x00") {
		if rel == "" {
			continue
		}
		hash, e := DigestFile(filepath.Join(source, rel))
		if e != nil {
			return e
		}
		fmt.Fprintf(&inputs, "%s %s\n", rel, hash)
	}
	buildDir := filepath.Join(root, ".wasmbench", "build")
	if e = os.MkdirAll(buildDir, 0755); e != nil {
		return e
	}
	modfile := filepath.Join(buildDir, "wago.mod")
	content := fmt.Sprintf("module github.com/wasmbench/wasmbench/adapters/wago\n\ngo 1.26.0\n\nrequire (\n github.com/wago-org/wago v0.0.0\n github.com/wago-org/wasi v0.3.1\n github.com/wago-org/component-model v0.1.6\n github.com/wasmbench/wasmbench v0.0.0\n)\nreplace github.com/wago-org/wago => %s\nreplace github.com/wasmbench/wasmbench => %s\n", strconv.Quote(source), strconv.Quote(root))
	if e = os.WriteFile(modfile, []byte(content), 0644); e != nil {
		return e
	}
	identity := strings.TrimSpace(string(revision)) + "/source-" + corpus.Hash([]byte(inputs.String()))
	cmd = exec.CommandContext(ctx, "go", "build", "-mod=mod", "-modfile="+modfile, "-trimpath", "-ldflags=-X main.sourceRevision="+identity, "-o", NativeExecutable(filepath.Join(root, "bin", "adapter-wago")), ".")
	cmd.Dir = filepath.Join(root, "adapters", "wago")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
