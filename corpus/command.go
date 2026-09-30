package corpus

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/wasmbench/wasmbench/protocol"
)

// Unknown command semantics stay unsupported. A supported contract's missing or
// changed input, however, is an import error, never a silently weakened oracle.
func importWagoCommand(base, id string, raw json.RawMessage, w *protocol.Workload) error {
	var c struct {
		Runtime   string            `json:"runtime"`
		Reference string            `json:"reference_runtime"`
		Platforms []string          `json:"platforms"`
		Export    string            `json:"export"`
		Argv0     string            `json:"argv0"`
		Args      []string          `json:"args"`
		Stdin     string            `json:"stdin"`
		Preopen   string            `json:"preopen"`
		Inputs    map[string]string `json:"inputs"`
		Normalize string            `json:"stdout_normalize"`
		Stdout    string            `json:"stdout_sha256"`
		Stderr    string            `json:"stderr_sha256"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil || (c.Runtime != "wasi" && c.Runtime != "emscripten") || (c.Runtime == "wasi" && c.Export != "_start") || (c.Runtime == "emscripten" && c.Export != "main") || (c.Stdout == "" && c.Stderr == "") {
		return nil
	}
	if !protocol.CommandStdoutNormalizerSupported(c.Normalize) {
		w.UnsupportedReason = "unsupported stdout normalization: " + c.Normalize
		return nil
	}
	if c.Argv0 == "" {
		c.Argv0 = id
	}
	cmd := &protocol.CommandContract{Argv: append([]string{c.Argv0}, c.Args...), StdoutSHA256: c.Stdout, StdoutNormalize: c.Normalize, StderrSHA256: c.Stderr, OutputLimit: 16 << 20, Files: map[string]protocol.CommandFile{}}
	if c.Preopen != "" && (!fs.ValidPath(c.Preopen) || strings.ContainsAny(c.Preopen, "\\\x00")) {
		return fmt.Errorf("unsafe command preopen")
	}
	if len(c.Inputs) > 0 && c.Preopen == "" {
		return fmt.Errorf("command inputs without preopen")
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return err
	}
	defer root.Close()
	var total int64
	for name, digest := range c.Inputs {
		if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "\\\x00") {
			return fmt.Errorf("unsafe command fixture path")
		}
		f, err := root.Open(path.Join(c.Preopen, name))
		if err != nil {
			return err
		}
		info, err := f.Stat()
		if err != nil {
			f.Close()
			return err
		}
		if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > protocol.CommandFileLimit-total {
			f.Close()
			return fmt.Errorf("command fixture size/type exceeds budget")
		}
		h := sha256.New()
		n, err := io.Copy(h, io.LimitReader(f, info.Size()+1))
		f.Close()
		if err != nil {
			return err
		}
		total += n
		if n != info.Size() || hex.EncodeToString(h.Sum(nil)) != digest {
			return fmt.Errorf("command fixture digest mismatch: %s", name)
		}
		absolute, err := filepath.Abs(filepath.Join(base, filepath.FromSlash(path.Join(c.Preopen, name))))
		if err != nil {
			return err
		}
		cmd.Files[name] = protocol.CommandFile{Path: absolute, Size: uint64(n), SHA256: digest}
	}
	if c.Stdin != "" {
		_, ok := cmd.Files[c.Stdin]
		if !ok {
			return fmt.Errorf("command stdin is not a pinned input")
		}
		cmd.StdinFile = c.Stdin
	}
	candidate := *w
	candidate.Command = cmd
	candidate.Export = c.Export
	if c.Runtime == "emscripten" {
		candidate.ABI = "emscripten"
		candidate.HostProfile = protocol.EmscriptenStdioProfile
	} else {
		candidate.ABI = "wasi-command"
		candidate.HostProfile = "wasi-preview1-readonly-v1"
	}
	candidate.WorkUnit = "command"
	candidate.Oracle = protocol.Oracle{Kind: "exact_command"}
	candidate.UnsupportedReason = ""
	if err := protocol.ValidateCommand(candidate); err != nil {
		if strings.Contains(err.Error(), "byte budget exceeded") {
			w.UnsupportedReason = "command input metadata or referenced files exceed their safety budget"
			return nil
		}
		return err
	}
	*w = candidate
	return nil
}
