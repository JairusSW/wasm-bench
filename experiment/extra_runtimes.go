package experiment

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/wasmbench/wasmbench/protocol"
)

// Shell paths are resolved before locking; changing PATH later cannot replace
// the engine in a locked experiment. No installer or jsvu is required.
func extraRuntimeCommand(root, id string) ([]string, bool, error) {
	tools := map[string]struct{ env, executable string }{
		"spidermonkey": {"WASMBENCH_SPIDERMONKEY", "spidermonkey"},
		"jsc":          {"WASMBENCH_JSC", "jsc"},
		"v8-shell":     {"WASMBENCH_V8_SHELL", "d8"},
		"deno":         {"WASMBENCH_DENO", "deno"},
	}
	if tool, ok := tools[id]; ok {
		path := os.Getenv(tool.env)
		if path == "" {
			if id == "jsc" && runtime.GOOS == "darwin" {
				path = NativeExecutable(filepath.Join(root, "bin", "adapter-jsc"))
			} else {
				path = tool.executable
			}
		}
		path, err := exec.LookPath(path)
		if err != nil {
			return nil, true, fmt.Errorf("%s: install %s or set %s to its executable: %w", id, tool.executable, tool.env, err)
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return nil, true, err
		}
		digest, err := DigestFile(path)
		if err != nil {
			return nil, true, err
		}
		script := filepath.Join(root, "adapters", "js-shell", "adapter.js")
		args := []string{path}
		switch id {
		case "deno":
			args = append(args, "run", "--no-prompt", "--allow-read", script)
		case "spidermonkey":
			args = append(args, "-f", script, "--")
		default:
			args = append(args, script, "--")
		}
		return append(args, "runtime="+id, "binary-sha256="+digest), true, nil
	}
	switch id {
	case "wasmi", "wavm", "wasm3", "wasmedge":
		return []string{NativeExecutable(filepath.Join(root, "bin", "adapter-"+id))}, true, nil
	}
	return nil, false, nil
}

func validateExtraProbe(data []byte) error {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		return fmt.Errorf("expected two JSON protocol responses, got %d", len(lines))
	}
	for i, line := range lines {
		var r protocol.Response
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return fmt.Errorf("invalid shell control response: %w", err)
		}
		if r.Version != protocol.Version || r.ID != i+1 || r.Status != "ok" || (i == 0 && (r.Description == nil || r.Description.Runtime == "")) {
			return fmt.Errorf("shell protocol probe failed: %s", line)
		}
	}
	return nil
}

// BuildExtraRuntime uses an installed shell or explicitly selected native SDK.
// It never silently falls back to another runtime or downloads SDKs.
func BuildExtraRuntime(ctx context.Context, root, id string) (bool, error) {
	if !slices.Contains([]string{"wasmi", "wavm", "wasm3", "wasmedge", "spidermonkey", "jsc", "v8-shell", "deno"}, id) {
		return false, nil
	}
	switch id {
	case "wasmi", "wavm", "wasm3", "wasmedge":
		cmd := exec.CommandContext(ctx, "cargo", "build", "--release", "--locked", "--no-default-features", "--features", id, "--target-dir", filepath.Join(root, "adapters", "native", "target", id))
		cmd.Dir = filepath.Join(root, "adapters", "native")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return true, err
		}
		source := NativeExecutable(filepath.Join(root, "adapters", "native", "target", id, "release", "adapter-native"))
		data, err := os.ReadFile(source)
		if err != nil {
			return true, err
		}
		if err = os.MkdirAll(filepath.Join(root, "bin"), 0755); err != nil {
			return true, err
		}
		return true, os.WriteFile(NativeExecutable(filepath.Join(root, "bin", "adapter-"+id)), data, 0755)
	default:
		if id == "jsc" && runtime.GOOS == "darwin" && os.Getenv("WASMBENCH_JSC") == "" {
			if err := os.MkdirAll(filepath.Join(root, "bin"), 0755); err != nil {
				return true, err
			}
			cmd := exec.CommandContext(ctx, "c++", "-std=c++17", "-O2", "-framework", "JavaScriptCore", filepath.Join(root, "adapters", "js-shell", "jsc-host.cpp"), "-o", filepath.Join(root, "bin", "adapter-jsc"))
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			if err := cmd.Run(); err != nil {
				return true, err
			}
		}
		command, _, err := extraRuntimeCommand(root, id)
		if err != nil {
			return true, err
		}
		// Check actual protocol support, not just executable presence or JS syntax.
		probeContext, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(probeContext, command[0], command[1:]...)
		cmd.Stdin = strings.NewReader("{\"version\":1,\"id\":1,\"method\":\"describe\"}\n{\"version\":1,\"id\":2,\"method\":\"close\"}\n")
		cmd.Stderr = os.Stderr
		data, err := cmd.Output()
		if err != nil {
			return true, fmt.Errorf("%s protocol probe: %w", id, err)
		}
		return true, validateExtraProbe(data)
	}
}
