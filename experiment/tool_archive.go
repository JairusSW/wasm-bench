package experiment

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Tools are retained as data, not executable files. No archive consumer launches
// code during inspection, verification, or restoration. SHA-256 is integrity,
// not a signature or a claim that somebody else's bundle is trusted.
type archivedTool struct {
	Source     string
	Relative   string
	SHA256     string
	Executable bool
	Runtime    int // -1 runner, -2 analyzer
	HostOnly   bool
}

func toolArchiveSpec(l Lock) ([]archivedTool, error) {
	entries := []archivedTool{{Relative: "tools/runner/wasmbench", SHA256: l.RunnerSHA256, Executable: true, Runtime: -1}}
	if l.Analyzer != nil {
		entries = append(entries, archivedTool{Source: l.Analyzer.Executable, Relative: "tools/analyzer/wasm-analyze", SHA256: l.Analyzer.SHA256, Executable: true, Runtime: -2})
	}
	for i, r := range l.Runtimes {
		if len(r.Command) == 0 || r.Files[r.Command[0]] == "" {
			return nil, fmt.Errorf("runtime %s executable is not pinned", r.ID)
		}
		keys := make([]string, 0, len(r.Files))
		for source := range r.Files {
			keys = append(keys, source)
		}
		sort.Strings(keys)
		relativePaths, err := archiveRelativePaths(keys)
		if err != nil {
			return nil, err
		}
		for _, source := range keys {
			if !recordedPathAbsolute(source) {
				return nil, fmt.Errorf("tool archive requires clean absolute paths: %q", source)
			}
			// Preserve relative imports without accumulating restoration prefixes
			// when a restored experiment is itself archived and replayed.
			rel := relativePaths[source]
			if !filepath.IsLocal(rel) {
				return nil, fmt.Errorf("invalid tool path %q", source)
			}
			rel = filepath.Join("tools", fmt.Sprintf("runtime-%d", i), "tree", rel)
			entries = append(entries, archivedTool{Source: source, Relative: filepath.ToSlash(rel), SHA256: r.Files[source], Executable: source == r.Command[0], Runtime: i})
		}
		for _, arg := range r.Command {
			if recordedPathLooksAbsolute(arg) && (!recordedPathAbsolute(arg) || r.Files[arg] == "") {
				return nil, fmt.Errorf("runtime %s has unpinned command file %q", r.ID, arg)
			}
		}
		hostKeys := make([]string, 0, len(r.HostFiles))
		for source := range r.HostFiles {
			hostKeys = append(hostKeys, source)
		}
		sort.Strings(hostKeys)
		for j, source := range hostKeys {
			if !recordedPathAbsolute(source) {
				return nil, fmt.Errorf("invalid host library path %q", source)
			}
			base, err := recordedPathBase(source)
			if err != nil {
				return nil, err
			}
			entries = append(entries, archivedTool{Source: source, Relative: filepath.ToSlash(filepath.Join("tools", fmt.Sprintf("runtime-%d", i), "host", fmt.Sprintf("%d", j), base)), SHA256: r.HostFiles[source], Runtime: i, HostOnly: true})
		}
	}
	if len(entries) > 4096 {
		return nil, fmt.Errorf("tool archive exceeds 4096 files")
	}
	for _, entry := range entries {
		digest, err := hex.DecodeString(entry.SHA256)
		if err != nil || len(digest) != 32 || strings.ToLower(entry.SHA256) != entry.SHA256 {
			return nil, fmt.Errorf("invalid tool SHA-256")
		}
	}
	return entries, nil
}

// Native interpreters can be installed on a different Windows volume from their
// scripts. Each volume gets its own tree; relative script imports are preserved.
// Single-volume archive paths retain the historical layout unchanged.
func archiveRelativePaths(keys []string) (map[string]string, error) {
	return portableArchiveRelativePaths(keys)
}

func archiveTools(l Lock, runner, out string) error {
	entries, err := toolArchiveSpec(l)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		source := entry.Source
		if entry.Runtime == -1 {
			source = runner
		}
		if err := linkCachedTool(source, filepath.Join(out, entry.Relative), entry.SHA256); err != nil {
			return err
		}
	}
	return nil
}

func copyTool(source, destination, digest string, executable bool) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("tool is not a regular file: %s", source)
	}
	if err := CopyExclusive(source, destination); err != nil {
		return err
	}
	got, err := DigestFile(destination)
	if err != nil {
		return err
	}
	if got != digest {
		return fmt.Errorf("tool changed while copying: %s", source)
	}
	mode := os.FileMode(0444)
	if executable {
		mode = 0555
	}
	return os.Chmod(destination, mode)
}

func verifyArchivedTools(l Lock, root string) error {
	if !l.ArchiveTools {
		return nil
	}
	entries, err := toolArchiveSpec(l)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		got, err := DigestFile(filepath.Join(root, entry.Relative))
		if err != nil {
			return fmt.Errorf("archived tool: %w", err)
		}
		if got != entry.SHA256 {
			return fmt.Errorf("archived tool differs from lock: %s", entry.Relative)
		}
	}
	return nil
}

type ToolRestoration struct {
	Version               string `json:"version"`
	SourceBundle          string `json:"source_bundle"`
	SourceChecksumsSHA256 string `json:"source_checksums_sha256"`
	SourceLockSHA256      string `json:"source_lock_sha256"`
	Runner                string `json:"runner"`
	Lock                  string `json:"lock"`
	Scope                 string `json:"scope"`
}

// RestoreTools exports exact recorded tools and workloads into a new directory.
// Relocation deliberately produces a new path-bound lock; it does not claim the
// original configuration identity, host qualification, or pilot confirmation.
func RestoreTools(bundleRoot, out string) (ToolRestoration, error) {
	var record ToolRestoration
	root, err := filepath.Abs(bundleRoot)
	if err != nil {
		return record, err
	}
	b, err := Load(root)
	if err != nil {
		return record, err
	}
	l := b.Manifest.Lock
	if !l.ArchiveTools {
		return record, fmt.Errorf("bundle has no tool archive; exact tools must be recovered separately")
	}
	if len(l.PilotPlan) != 0 {
		return record, fmt.Errorf("pilot confirmation locks cannot be relocated: use original tool paths to preserve the fixed confirmation identity")
	}
	if b.Manifest.Host.OS != runtime.GOOS || b.Manifest.Host.Arch != runtime.GOARCH {
		return record, fmt.Errorf("tool restoration requires the recorded OS and architecture")
	}
	out, err = filepath.Abs(out)
	if err != nil {
		return record, err
	}
	if rel, e := filepath.Rel(root, out); e == nil && (rel == "." || filepath.IsLocal(rel)) {
		return record, fmt.Errorf("restoration output must be outside the immutable source bundle")
	}
	digest, err := DigestFile(filepath.Join(root, "checksums.json"))
	if err != nil {
		return record, err
	}
	entries, err := toolArchiveSpec(l)
	if err != nil {
		return record, err
	}
	if err = os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return record, err
	}
	if err = os.Mkdir(out, 0755); err != nil {
		return record, err
	}
	record = ToolRestoration{Version: "exact-tool-restoration-v1", SourceBundle: root, SourceChecksumsSHA256: digest, SourceLockSHA256: b.Manifest.LockSHA256, Runner: filepath.Join(out, "tools/runner/wasmbench"), Lock: filepath.Join(out, "suite.lock"), Scope: "exact pinned files; relocated path-bound lock; absolute native libraries remain hash-verified host prerequisites; OS shared-cache libraries and unlisted dependencies excluded; no code executed"}
	replacements := make([]map[string]string, len(l.Runtimes))
	record.Runner = NativeExecutable(record.Runner)
	for i := range replacements {
		replacements[i] = map[string]string{}
	}
	for _, entry := range entries {
		dst := filepath.Join(out, entry.Relative)
		if entry.Executable {
			dst = NativeExecutable(dst)
		}
		if err = copyTool(filepath.Join(root, entry.Relative), dst, entry.SHA256, entry.Executable); err != nil {
			return record, err
		}
		if entry.Runtime == -2 {
			l.Analyzer.Executable = dst
		}
		if entry.Runtime >= 0 && !entry.HostOnly {
			replacements[entry.Runtime][entry.Source] = dst
		}
	}
	for i := range l.Runtimes {
		r := &l.Runtimes[i]
		files := map[string]string{}
		for source, sha := range r.Files {
			files[replacements[i][source]] = sha
		}
		for j, arg := range r.Command {
			if dst, ok := replacements[i][arg]; ok {
				r.Command[j] = dst
			}
		}
		r.Files = files
	}
	for i := range l.Workloads {
		w := &l.Workloads[i]
		if !filepath.IsLocal(w.Artifact) {
			return record, fmt.Errorf("nonportable workload artifact")
		}
		dst := filepath.Join(out, w.Artifact)
		if _, err = os.Stat(dst); os.IsNotExist(err) {
			if err = CopyExclusive(filepath.Join(root, w.Artifact), dst); err != nil {
				return record, err
			}
		}
		if err = bundleCommandFiles(w, root, out); err != nil {
			return record, err
		}
	}
	if err = VerifyInputs(l, out); err != nil {
		return record, err
	}
	if err = WriteJSON(record.Lock, l); err != nil {
		return record, err
	}
	return record, WriteJSON(filepath.Join(out, "restoration.json"), record)
}
