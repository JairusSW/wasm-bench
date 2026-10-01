package experiment

import (
	"debug/macho"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Only Mach-O loader resolution is qualified here. System shared-cache images
// remain OS prerequisites; third-party absolute loads are exact host files, not
// silently rebound to a different dylib with the same basename.
func pinNativeDependencies(r *Runtime) error {
	if len(r.Command) == 0 {
		return fmt.Errorf("empty command")
	}
	_, closeFile, err := openNativeMachO(r.Command[0])
	if err != nil {
		if errors.Is(err, errNoNativeMachOSlice) {
			return err
		}
		return pinELFDependencies(r)
	} // ELF is qualified separately; other formats retain the declared-file contract.
	closeFile()
	r.NativeDependencyPolicy = "macho-third-party-closure-v1; system shared-cache libraries excluded; absolute loads remain exact host prerequisites"
	r.HostFiles = map[string]string{}
	seen := map[string]bool{}
	mainDir := filepath.Dir(r.Command[0])
	expand := func(value, loader string) string {
		if value == "@loader_path" {
			return filepath.Dir(loader)
		}
		if value == "@executable_path" {
			return mainDir
		}
		if strings.HasPrefix(value, "@loader_path/") {
			return filepath.Join(filepath.Dir(loader), strings.TrimPrefix(value, "@loader_path/"))
		}
		if strings.HasPrefix(value, "@executable_path/") {
			return filepath.Join(mainDir, strings.TrimPrefix(value, "@executable_path/"))
		}
		return value
	}
	type searchPath struct {
		path        string
		relocatable bool
	}
	var visit func(string, bool, []searchPath) error
	visit = func(path string, relocated bool, inherited []searchPath) error {
		key := fmt.Sprintf("%t:%s", relocated, path)
		if seen[key] {
			return nil
		}
		if len(seen) >= 4096 {
			return fmt.Errorf("native dependency closure exceeds 4096 files")
		}
		seen[key] = true
		m, closeFile, err := openNativeMachO(path)
		if err != nil {
			return err
		}
		defer closeFile()
		search := []searchPath{}
		for _, load := range m.Loads {
			if rp, ok := load.(*macho.Rpath); ok {
				search = append(search, searchPath{expand(rp.Path, path), strings.HasPrefix(rp.Path, "@loader_path") || strings.HasPrefix(rp.Path, "@executable_path")})
			}
		}
		search = append(search, inherited...)
		imports, err := m.ImportedLibraries()
		if err != nil {
			return err
		}
		for _, name := range imports {
			// These are supplied by the compatible macOS shared cache, not ordinary
			// distributable files. Do not pretend a missing on-disk image is zero bytes.
			if strings.HasPrefix(name, "/usr/lib/") || strings.HasPrefix(name, "/System/Library/") {
				continue
			}
			target := expand(name, path)
			canRelocate := strings.HasPrefix(name, "@loader_path/") || strings.HasPrefix(name, "@executable_path/")
			if strings.HasPrefix(name, "@rpath/") {
				target = ""
				for _, dir := range search {
					candidate := filepath.Join(dir.path, strings.TrimPrefix(name, "@rpath/"))
					if info, e := os.Stat(candidate); e == nil && info.Mode().IsRegular() {
						target = candidate
						canRelocate = dir.relocatable
						break
					}
				}
				if target == "" {
					return fmt.Errorf("unresolved native dependency %s from %s", name, path)
				}
			}
			if !filepath.IsAbs(target) {
				return fmt.Errorf("unsupported native load path %s", name)
			}
			target = filepath.Clean(target)
			digest, err := DigestFile(target)
			if err != nil {
				return err
			}
			moves := relocated && canRelocate
			if moves {
				r.Files[target] = digest
			} else {
				r.HostFiles[target] = digest
			}
			if err = visit(target, moves, search); err != nil {
				return err
			}
		}
		return nil
	}
	return visit(r.Command[0], true, nil)
}

var errNoNativeMachOSlice = errors.New("universal Mach-O has no native slice")

// Universal SpiderMonkey shells and their dylibs must pin the native slice's
// startup dependency closure too. Hashing only the fat executable is not enough.
func openNativeMachO(path string) (*macho.File, func() error, error) {
	if file, err := macho.Open(path); err == nil {
		return file, file.Close, nil
	}
	fat, err := macho.OpenFat(path)
	if err != nil {
		return nil, nil, err
	}
	wanted := macho.Cpu(0)
	switch runtime.GOARCH {
	case "arm64":
		wanted = macho.CpuArm64
	case "amd64":
		wanted = macho.CpuAmd64
	}
	for _, arch := range fat.Arches {
		if arch.Cpu == wanted {
			return arch.File, fat.Close, nil
		}
	}
	fat.Close()
	return nil, nil, fmt.Errorf("%w for %s", errNoNativeMachOSlice, runtime.GOARCH)
}
