package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const EmscriptenStdioProfile = "emscripten-stdio-v1"

// CommandContract pins inline or file-backed fixtures so locked runs never
// expose an ambient host directory. Component temporary-filesystem workloads
// may mutate only their disposable per-sample staging root.
// Argv includes argv[0]. Empty stream digests explicitly mean unchecked.
type CommandContract struct {
	Argv            []string               `json:"argv"`
	Stdin           []byte                 `json:"stdin,omitempty"`
	StdinFile       string                 `json:"stdin_file,omitempty"`
	Files           map[string]CommandFile `json:"files,omitempty"`
	ExitCode        uint32                 `json:"exit_code"`
	StdoutSHA256    string                 `json:"stdout_sha256,omitempty"`
	StdoutNormalize string                 `json:"stdout_normalize,omitempty"`
	StderrSHA256    string                 `json:"stderr_sha256,omitempty"`
	OutputLimit     uint64                 `json:"output_limit_bytes"`
}
type CommandFile struct {
	Data   []byte `json:"data,omitempty"`
	Path   string `json:"path,omitempty"`
	Size   uint64 `json:"size,omitempty"`
	SHA256 string `json:"sha256"`
}
type CommandResult struct {
	ExitCode           uint32 `json:"exit_code"`
	StdoutSHA256       string `json:"stdout_sha256"`
	StdoutOracleSHA256 string `json:"stdout_oracle_sha256,omitempty"`
	StderrSHA256       string `json:"stderr_sha256"`
	StdoutBytes        uint64 `json:"stdout_bytes"`
	StderrBytes        uint64 `json:"stderr_bytes"`
}

const CommandInputLimit = 1 << 20
const CommandFileLimit = 256 << 20

func CommandDigest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

var llvmIRPredPadding = regexp.MustCompile(` +; preds =`)

// CommandStdoutNormalizerSupported is an allowlist: unknown corpus normalizers
// remain visible as unsupported instead of being treated as raw-output checks.
func CommandStdoutNormalizerSupported(name string) bool {
	return name == "" || name == "llvm-ir-preds"
}

// NormalizeCommandStdout implements only versioned, explicitly named output
// normalizers. Raw output evidence remains separate from this oracle view.
func NormalizeCommandStdout(name string, data []byte) ([]byte, error) {
	switch name {
	case "":
		return data, nil
	case "llvm-ir-preds":
		return llvmIRPredPadding.ReplaceAll(data, []byte(" ; preds =")), nil
	default:
		return nil, fmt.Errorf("unsupported stdout normalization %q", name)
	}
}

func validDigest(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32 && s == strings.ToLower(s)
}

func ValidateCommand(w Workload) error {
	c := w.Command
	hostProfileOK := w.ABI == "wasi-command" && w.HostProfile == "wasi-preview1-readonly-v1" || w.ABI == "emscripten" && w.HostProfile == EmscriptenStdioProfile || w.ABI == "component" && (w.HostProfile == "wasi-preview2-readonly-v1" || w.HostProfile == "wasi-preview2-temporary-filesystem-v1")
	exportOK := w.ABI == "emscripten" && w.Export == "main" || w.ABI != "emscripten" && w.Export == "_start"
	if c == nil || !hostProfileOK || !exportOK || w.Oracle.Kind != "exact_command" || w.Reset != "fresh_instance_per_sample" || w.Input != nil || w.Vectors != nil || len(w.Args) != 0 || len(w.Oracle.Expected) != 0 || len(w.Oracle.Memory) != 0 || w.Initialize != "" || w.Oracle.OutputPointerExport != "" {
		return fmt.Errorf("unsupported or ambiguous command contract")
	}
	if len(c.Argv) == 0 || len(c.Argv) > 4096 || len(c.Files) > 1024 || c.Argv[0] == "" || c.OutputLimit == 0 || c.OutputLimit > 64<<20 {
		return fmt.Errorf("invalid command argv/output budget")
	}
	if w.ABI == "component" && c.ExitCode > 1 {
		return fmt.Errorf("WASI Preview 2 command exit oracle must be 0 or 1")
	}
	if c.StdoutSHA256 == "" && c.StderrSHA256 == "" {
		return fmt.Errorf("command needs an exact output oracle")
	}
	if !CommandStdoutNormalizerSupported(c.StdoutNormalize) {
		return fmt.Errorf("unsupported stdout normalization %q", c.StdoutNormalize)
	}
	for _, h := range []string{c.StdoutSHA256, c.StderrSHA256} {
		if h != "" && !validDigest(h) {
			return fmt.Errorf("invalid command stream digest")
		}
	}
	total := uint64(len(c.Stdin))
	var external uint64
	if c.StdinFile != "" {
		if _, ok := c.Files[c.StdinFile]; !ok || len(c.Stdin) != 0 {
			return fmt.Errorf("invalid or ambiguous command stdin file")
		}
	}
	for _, arg := range c.Argv {
		if strings.ContainsRune(arg, 0) {
			return fmt.Errorf("NUL in command argv")
		}
		total += uint64(len(arg))
	}
	for name, file := range c.Files {
		if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "\\\x00") {
			return fmt.Errorf("invalid command fixture path %q", name)
		}
		if !validDigest(file.SHA256) || (file.Path == "" && CommandDigest(file.Data) != file.SHA256) {
			return fmt.Errorf("command fixture digest mismatch: %s", name)
		}
		if file.Path != "" {
			if len(file.Data) != 0 || (!filepath.IsAbs(file.Path) && !filepath.IsLocal(file.Path)) || strings.ContainsRune(file.Path, 0) || file.Size > CommandFileLimit {
				return fmt.Errorf("invalid command file reference")
			}
			external += file.Size
		} else if file.Size != 0 && file.Size != uint64(len(file.Data)) {
			return fmt.Errorf("invalid inline command file size")
		}
		total += uint64(len(name) + len(file.Data) + len(file.Path))
		for prefix := name; strings.Contains(prefix, "/"); {
			prefix = prefix[:strings.LastIndexByte(prefix, '/')]
			if _, ok := c.Files[prefix]; ok {
				return fmt.Errorf("command file/directory collision")
			}
		}
	}
	if total > CommandInputLimit || external > CommandFileLimit {
		return fmt.Errorf("command input byte budget exceeded")
	}
	return nil
}

// CopyCommandFile streams a pinned regular file, verifying both length and
// digest. The caller must discard a destination if verification fails.
func CopyCommandFile(dst io.Writer, file CommandFile, base string) error {
	if file.Path == "" {
		if CommandDigest(file.Data) != file.SHA256 {
			return fmt.Errorf("command fixture digest mismatch")
		}
		_, err := dst.Write(file.Data)
		return err
	}
	if file.Size > CommandFileLimit || !validDigest(file.SHA256) || len(file.Data) != 0 {
		return fmt.Errorf("invalid command file reference")
	}
	name := file.Path
	if !filepath.IsAbs(name) {
		if !filepath.IsLocal(name) {
			return fmt.Errorf("unsafe command file path")
		}
		name = filepath.Join(base, name)
	}
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || uint64(info.Size()) != file.Size {
		return fmt.Errorf("command fixture size/type mismatch")
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(dst, h), io.LimitReader(f, int64(file.Size)+1))
	if err != nil {
		return err
	}
	if uint64(n) != file.Size || hex.EncodeToString(h.Sum(nil)) != file.SHA256 {
		return fmt.Errorf("command fixture digest/size mismatch")
	}
	return nil
}

func (c *CommandContract) Verify(result CommandResult) error {
	if !validDigest(result.StdoutSHA256) || !validDigest(result.StderrSHA256) || result.StdoutBytes > c.OutputLimit || result.StderrBytes > c.OutputLimit {
		return fmt.Errorf("incorrect result: invalid command stream evidence")
	}
	stdoutOracle := result.StdoutSHA256
	if c.StdoutNormalize != "" {
		if !validDigest(result.StdoutOracleSHA256) {
			return fmt.Errorf("incorrect result: missing normalized stdout evidence")
		}
		stdoutOracle = result.StdoutOracleSHA256
	} else if result.StdoutOracleSHA256 != "" {
		return fmt.Errorf("incorrect result: unexpected normalized stdout evidence")
	}
	if result.ExitCode != c.ExitCode {
		return fmt.Errorf("incorrect result: command exit=%d expected=%d", result.ExitCode, c.ExitCode)
	}
	if c.StdoutSHA256 != "" && stdoutOracle != c.StdoutSHA256 {
		return fmt.Errorf("incorrect result: command stdout oracle mismatch (normalizer=%q raw_sha256=%s oracle_sha256=%s expected_sha256=%s)", c.StdoutNormalize, result.StdoutSHA256, stdoutOracle, c.StdoutSHA256)
	}
	if c.StderrSHA256 != "" && result.StderrSHA256 != c.StderrSHA256 {
		return fmt.Errorf("incorrect result: command stderr mismatch (actual_sha256=%s expected_sha256=%s)", result.StderrSHA256, c.StderrSHA256)
	}
	return nil
}
