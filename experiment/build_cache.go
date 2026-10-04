package experiment

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/wasmbench/wasmbench/corpus"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type adapterBuildReceipt struct {
	Key    string `json:"key"`
	SHA256 string `json:"sha256"`
}

// The key includes dirty tracked and untracked Go inputs, dependency declarations,
// source identity and the complete Go toolchain configuration. A changed input
// cannot inherit a previous adapter's source identity.
func cachedWagoBuild(ctx context.Context, root, source, identity, modfile string, build func(string) error) error {
	var inputs strings.Builder
	inputs.WriteString(identity)
	inputs.WriteString(modfile)
	for _, tree := range []string{root, source} {
		cmd := exec.CommandContext(ctx, "git", "-C", tree, "ls-files", "--cached", "--others", "--exclude-standard", "-z", "*.go", "*.s", "*.S", "go.mod", "go.sum")
		paths, err := cmd.Output()
		if err != nil {
			return err
		}
		for _, path := range strings.Split(string(paths), "\x00") {
			if path == "" {
				continue
			}
			digest, err := DigestFile(filepath.Join(tree, path))
			if err != nil {
				return err
			}
			fmt.Fprintf(&inputs, "\n%s/%s %s", tree, path, digest)
		}
	}
	cmd := exec.CommandContext(ctx, "go", "env", "-json", "GOOS", "GOARCH", "GOVERSION", "GOTOOLCHAIN", "GOAMD64", "GOARM64", "GOARM", "GO386", "GOMIPS", "GOMIPS64", "GOPPC64", "GORISCV64", "GOWASM", "GOEXPERIMENT", "CGO_ENABLED", "CC", "CXX", "CGO_CFLAGS", "CGO_CPPFLAGS", "CGO_CXXFLAGS", "CGO_FFLAGS", "CGO_LDFLAGS", "GOFLAGS", "GOROOT")
	env, err := cmd.Output()
	if err != nil {
		return err
	}
	inputs.Write(env)
	key := corpus.Hash([]byte(inputs.String()))
	cache := os.Getenv("WASMBENCH_TOOL_CACHE")
	if cache == "" {
		cache, err = os.UserCacheDir()
		if err != nil {
			return err
		}
		cache = filepath.Join(cache, "wasm-bench", "tools")
	}
	directory := filepath.Join(cache, "builds", "wago", key)
	binary := NativeExecutable(filepath.Join(directory, "adapter-wago"))
	receiptPath := filepath.Join(directory, "build.json")
	var receipt adapterBuildReceipt
	if err := ReadJSON(receiptPath, &receipt); err == nil {
		digest, err := DigestFile(binary)
		if err != nil {
			return err
		}
		if receipt.Key != key || receipt.SHA256 != digest {
			return fmt.Errorf("cached Wago adapter differs from build receipt")
		}
		fmt.Println("Wago adapter cache hit:", key)
	} else if !os.IsNotExist(err) {
		return err
	} else {
		if err := os.MkdirAll(directory, 0755); err != nil {
			return err
		}
		temp, err := os.CreateTemp(directory, ".adapter-*")
		if err != nil {
			return err
		}
		name := temp.Name()
		temp.Close()
		defer os.Remove(name)
		if err := build(name); err != nil {
			return err
		}
		digest, err := DigestFile(name)
		if err != nil {
			return err
		}
		if err := os.Chmod(name, 0555); err != nil {
			return err
		}
		if err := os.Link(name, binary); err != nil && !os.IsExist(err) {
			return err
		}
		got, err := DigestFile(binary)
		if err != nil {
			return err
		}
		if got != digest {
			return fmt.Errorf("concurrent Wago cache build differs")
		}
		receipt = adapterBuildReceipt{Key: key, SHA256: digest}
		raw, err := json.Marshal(receipt)
		if err != nil {
			return err
		}
		tempReceipt, err := os.CreateTemp(directory, ".receipt-*")
		if err != nil {
			return err
		}
		receiptName := tempReceipt.Name()
		defer os.Remove(receiptName)
		if _, err := tempReceipt.Write(raw); err != nil {
			tempReceipt.Close()
			return err
		}
		if err := tempReceipt.Close(); err != nil {
			return err
		}
		if err := os.Rename(receiptName, receiptPath); err != nil {
			return err
		}
	}
	destination := NativeExecutable(filepath.Join(root, "bin", "adapter-wago"))
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(destination), ".adapter-link-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	temp.Close()
	os.Remove(name)
	defer os.Remove(name)
	if err := os.Link(binary, name); err != nil {
		return fmt.Errorf("link adapter build cache: %w; keep WASMBENCH_TOOL_CACHE on the adapter filesystem", err)
	}
	return os.Rename(name, destination)
}
