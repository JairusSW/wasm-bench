package experiment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"
)

const elfDependencyVersion = "linux-glibc-startup-closure-v1"
const elfDependencyPolicy = "linux-glibc-startup-closure-v1; explicit trusted loader --list; exact startup library hashes and loader controls; re-resolved before execution; excludes later dlopen and kernel vDSO"

type NativeLibrary struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type ELFDependencies struct {
	Version      string                   `json:"version"`
	Interpreter  NativeLibrary            `json:"interpreter"`
	Libraries    map[string]NativeLibrary `json:"libraries"`
	ControlFiles map[string]string        `json:"control_files"` // SHA-256 or explicit absent.
}

func elfEnvironment() error {
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok && value != "" && (strings.HasPrefix(key, "LD_") || key == "GLIBC_TUNABLES") {
			return fmt.Errorf("ELF dependency qualification requires unset loader override %s", key)
		}
	}
	return nil
}

func elfControls() (map[string]string, error) {
	if err := elfEnvironment(); err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, path := range []string{"/etc/ld.so.cache", "/etc/ld.so.preload"} {
		file, err := os.Open(path)
		if os.IsNotExist(err) {
			result[path] = "absent"
			continue
		}
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(file, 16<<20+1))
		file.Close()
		if err != nil {
			return nil, err
		}
		if len(data) > 16<<20 {
			return nil, fmt.Errorf("ELF loader control file exceeds 16 MiB")
		}
		if path == "/etc/ld.so.preload" && len(bytes.TrimSpace(data)) != 0 {
			return nil, fmt.Errorf("nonempty system ELF preload is not qualified")
		}
		result[path] = hashBytes(data)
	}
	return result, nil
}

func hashBytes(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// This inspects ELF metadata only. The dynamic probe is separate so static
// binaries and format tests never invoke another executable.
func elfInterpreter(path string) (string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	var magic [4]byte
	_, readErr := f.Read(magic[:])
	f.Close()
	if readErr != nil || string(magic[:]) != "\x7fELF" {
		return "", false, nil
	}
	image, err := elf.Open(path)
	if err != nil {
		return "", true, err
	}
	defer image.Close()
	if image.Type != elf.ET_EXEC && image.Type != elf.ET_DYN {
		return "", true, fmt.Errorf("ELF runtime must be executable or PIE")
	}
	var interpreter string
	for _, p := range image.Progs {
		if p.Type != elf.PT_INTERP {
			continue
		}
		if interpreter != "" || p.Filesz < 2 || p.Filesz > 4096 {
			return "", true, fmt.Errorf("invalid ELF interpreter segment")
		}
		raw, err := io.ReadAll(io.LimitReader(p.Open(), 4097))
		if err != nil {
			return "", true, err
		}
		if len(raw) == 0 || raw[len(raw)-1] != 0 || bytes.ContainsRune(raw[:len(raw)-1], 0) {
			return "", true, fmt.Errorf("invalid ELF interpreter string")
		}
		interpreter = string(raw[:len(raw)-1])
	}
	needed, err := image.ImportedLibraries()
	if err != nil {
		return "", true, err
	}
	if interpreter == "" && len(needed) > 0 {
		return "", true, fmt.Errorf("ELF dependencies without a standard interpreter are not qualified")
	}
	for _, tag := range []elf.DynTag{elf.DT_AUDIT, elf.DT_DEPAUDIT} {
		values, err := image.DynValue(tag)
		if err != nil {
			return "", true, err
		}
		if len(values) > 0 {
			return "", true, fmt.Errorf("ELF audit modules are not qualified")
		}
	}
	if interpreter != "" {
		allowed := ""
		switch image.Machine {
		case elf.EM_AARCH64:
			allowed = "/lib/ld-linux-aarch64.so.1"
		case elf.EM_X86_64:
			allowed = "/lib64/ld-linux-x86-64.so.2"
		}
		if interpreter != allowed || allowed == "" {
			return "", true, fmt.Errorf("unsupported ELF interpreter %q", interpreter)
		}
		if runtime.GOOS != "linux" || (image.Machine == elf.EM_AARCH64 && runtime.GOARCH != "arm64") || (image.Machine == elf.EM_X86_64 && runtime.GOARCH != "amd64") {
			return "", true, fmt.Errorf("ELF loader probe requires matching native Linux architecture")
		}
	}
	return interpreter, true, nil
}

func parseELFList(output, interpreter string) (map[string]string, error) {
	libraries := map[string]string{}
	loaderSeen := false
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		at := strings.LastIndex(line, "(0x")
		if at < 0 || !strings.HasSuffix(line, ")") {
			return nil, fmt.Errorf("unrecognized or unresolved ELF loader entry %q", line)
		}
		address := line[at+3 : len(line)-1]
		if address == "" || strings.Trim(address, "0123456789abcdefABCDEF") != "" {
			return nil, fmt.Errorf("invalid ELF loader address")
		}
		entry := strings.TrimSpace(line[:at])
		// glibc may print the main executable as an unnamed address-only entry.
		if entry == "" || entry == "linux-vdso.so.1" {
			continue
		}
		if entry == interpreter {
			loaderSeen = true
			continue
		}
		name, path, ok := strings.Cut(entry, " => ")
		if !ok {
			return nil, fmt.Errorf("unrecognized ELF loader mapping %q", entry)
		}
		if name == "" || strings.ContainsAny(name, "\t\n\r") || !filepath.IsAbs(path) {
			return nil, fmt.Errorf("invalid ELF library mapping %q", entry)
		}
		if previous, ok := libraries[name]; ok && previous != path {
			return nil, fmt.Errorf("ambiguous ELF library %s", name)
		}
		libraries[name] = path
		if len(libraries) > 4096 {
			return nil, fmt.Errorf("ELF library limit exceeded")
		}
	}
	if !loaderSeen {
		return nil, fmt.Errorf("ELF loader omitted its interpreter mapping")
	}
	return libraries, nil
}

func probeELF(ctx context.Context, path string) (*ELFDependencies, error) {
	interpreter, recognized, err := elfInterpreter(path)
	if err != nil || !recognized {
		return nil, err
	}
	result := &ELFDependencies{Version: elfDependencyVersion, Libraries: map[string]NativeLibrary{}}
	if interpreter == "" {
		return result, nil
	}
	controls, err := elfControls()
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		return nil, fmt.Errorf("privileged ELF executables are not qualified")
	}
	digest, err := DigestFile(interpreter)
	if err != nil {
		return nil, err
	}
	result.Interpreter = NativeLibrary{Path: interpreter, SHA256: digest}
	result.ControlFiles = controls
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, interpreter, "--list", path)
	cmd.WaitDelay = time.Second
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out := &boundedAnalysisOutput{limit: 4 << 20}
	stderr := &boundedAnalysisOutput{limit: 64 << 10}
	cmd.Stdout = out
	cmd.Stderr = stderr
	if err = cmd.Run(); err != nil {
		return nil, fmt.Errorf("ELF loader dependency probe: %w: %s", err, stderr.String())
	}
	if stderr.String() != "" {
		return nil, fmt.Errorf("ELF loader diagnostics: %s", stderr.String())
	}
	paths, err := parseELFList(string(out.Bytes()), interpreter)
	if err != nil {
		return nil, err
	}
	for name, path := range paths {
		digest, err := DigestFile(path)
		if err != nil {
			return nil, err
		}
		clean := filepath.Clean(path)
		if clean != path {
			normalized, err := DigestFile(clean)
			if err != nil || normalized != digest {
				return nil, fmt.Errorf("ELF path normalization changes library identity: %s", path)
			}
		}
		result.Libraries[name] = NativeLibrary{Path: clean, SHA256: digest}
	}
	after, err := elfControls()
	if err != nil {
		return nil, err
	}
	now, err := DigestFile(interpreter)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(controls, after) || now != result.Interpreter.SHA256 {
		return nil, fmt.Errorf("ELF loader controls changed during qualification")
	}
	return result, nil
}

func pinELFDependencies(r *Runtime) error {
	probe, err := probeELF(context.Background(), r.Command[0])
	if err != nil || probe == nil {
		return err
	}
	r.ELF = probe
	r.NativeDependencyPolicy = elfDependencyPolicy
	if r.HostFiles == nil {
		r.HostFiles = map[string]string{}
	}
	if probe.Interpreter.Path != "" {
		r.HostFiles[probe.Interpreter.Path] = probe.Interpreter.SHA256
	}
	for path, digest := range probe.ControlFiles {
		if digest != "absent" {
			r.HostFiles[path] = digest
		}
	}
	for _, library := range probe.Libraries {
		r.Files[library.Path] = library.SHA256
	}
	return nil
}

func validateELFContract(r Runtime) error {
	if r.ELF == nil {
		if r.NativeDependencyPolicy == elfDependencyPolicy {
			return fmt.Errorf("ELF policy missing dependency evidence")
		}
		return nil
	}
	e := r.ELF
	if e.Version != elfDependencyVersion || r.NativeDependencyPolicy != elfDependencyPolicy || e.Libraries == nil || len(e.Libraries) > 4096 {
		return fmt.Errorf("invalid ELF dependency contract")
	}
	if e.Interpreter.Path == "" {
		if e.Interpreter.SHA256 != "" || len(e.Libraries) != 0 || len(e.ControlFiles) != 0 {
			return fmt.Errorf("invalid static ELF dependency contract")
		}
		return nil
	}
	if !recordedPOSIXPath(e.Interpreter.Path) || !validNativeDigest(e.Interpreter.SHA256) || r.HostFiles[e.Interpreter.Path] != e.Interpreter.SHA256 || len(e.ControlFiles) != 2 {
		return fmt.Errorf("ELF interpreter/controls not pinned")
	}
	for path, digest := range e.ControlFiles {
		if path != "/etc/ld.so.cache" && path != "/etc/ld.so.preload" {
			return fmt.Errorf("unknown ELF control file")
		}
		if digest != "absent" && (!validNativeDigest(digest) || r.HostFiles[path] != digest) {
			return fmt.Errorf("ELF control file not pinned")
		}
	}
	hashes := map[string]bool{}
	for _, digest := range r.Files {
		hashes[digest] = true
	}
	for name, library := range e.Libraries {
		if name == "" || !recordedPOSIXPath(library.Path) || !validNativeDigest(library.SHA256) || !hashes[library.SHA256] {
			return fmt.Errorf("ELF library %q is not pinned", name)
		}
	}
	return nil
}

func validNativeDigest(value string) bool {
	digest, err := hex.DecodeString(value)
	return err == nil && len(digest) == 32 && strings.ToLower(value) == value
}

func verifyELFControls(r Runtime) error {
	if err := validateELFContract(r); err != nil {
		return err
	}
	if r.ELF == nil || r.ELF.Interpreter.Path == "" {
		return nil
	}
	current, err := elfControls()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, r.ELF.ControlFiles) {
		return fmt.Errorf("runtime %s ELF loader controls differ from lock", r.ID)
	}
	return nil
}

func qualifyELFRuntime(ctx context.Context, r *Runtime) error {
	if r.ELF == nil {
		return nil
	}
	actual, err := probeELF(ctx, r.Command[0])
	if err != nil {
		return err
	}
	if actual == nil || actual.Version != r.ELF.Version || actual.Interpreter != r.ELF.Interpreter || !reflect.DeepEqual(actual.ControlFiles, r.ELF.ControlFiles) || len(actual.Libraries) != len(r.ELF.Libraries) {
		return fmt.Errorf("runtime %s ELF startup dependency set changed", r.ID)
	}
	for name, library := range actual.Libraries {
		if library.SHA256 != r.ELF.Libraries[name].SHA256 {
			return fmt.Errorf("runtime %s resolved different ELF library %s", r.ID, name)
		}
	}
	// Preserve actual resolved paths for this run, including $ORIGIN relocation.
	r.ELF = actual
	return nil
}
