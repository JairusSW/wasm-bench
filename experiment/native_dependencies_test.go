package experiment

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Minimal parseable Mach-O images test loader paths on every CI OS without
// executing foreign binaries or requiring a system compiler/linker.
func writeMachO(t *testing.T, path string, imports, rpaths []string) {
	t.Helper()
	var commands []byte
	add := func(command uint32, name string, offset int) {
		size := (offset + len(name) + 1 + 7) &^ 7
		b := make([]byte, size)
		binary.LittleEndian.PutUint32(b, command)
		binary.LittleEndian.PutUint32(b[4:], uint32(size))
		binary.LittleEndian.PutUint32(b[8:], uint32(offset))
		copy(b[offset:], name)
		commands = append(commands, b...)
	}
	for _, name := range imports {
		add(0xc, name, 24)
	}
	for _, path := range rpaths {
		add(0x8000001c, path, 12)
	}
	header := make([]byte, 32)
	for i, v := range []uint32{0xfeedfacf, 0x100000c, 0, 2, uint32(len(imports) + len(rpaths)), uint32(len(commands)), 0, 0} {
		binary.LittleEndian.PutUint32(header[i*4:], v)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(header, commands...), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestMachOPinsUniversalShellDependencies(t *testing.T) {
	if runtime.GOARCH != "arm64" && runtime.GOARCH != "amd64" {
		t.Skip("fixture covers supported native architectures")
	}
	dir := t.TempDir()
	slice := filepath.Join(dir, "slice")
	library := filepath.Join(dir, "libengine.dylib")
	writeMachO(t, library, nil, nil)
	writeMachO(t, slice, []string{"@rpath/libengine.dylib"}, []string{"@executable_path"})
	thin, err := os.ReadFile(slice)
	if err != nil {
		t.Fatal(err)
	}
	fat := make([]byte, 12288)
	binary.BigEndian.PutUint32(fat, 0xcafebabe)
	binary.BigEndian.PutUint32(fat[4:], 2)
	for i, cpu := range []uint32{0x100000c, 0x1000007} {
		offset := 4096 * (i + 1)
		arch := fat[8+i*20:]
		for j, v := range []uint32{cpu, 0, uint32(offset), uint32(len(thin)), 12} {
			binary.BigEndian.PutUint32(arch[j*4:], v)
		}
		copy(fat[offset:], thin)
		binary.LittleEndian.PutUint32(fat[offset+4:], cpu)
	}
	exe := filepath.Join(dir, "shell")
	if err = os.WriteFile(exe, fat, 0755); err != nil {
		t.Fatal(err)
	}
	digest, _ := DigestFile(exe)
	r := Runtime{Command: []string{exe}, Files: map[string]string{exe: digest}}
	if err = pinNativeDependencies(&r); err != nil {
		t.Fatal(err)
	}
	if r.Files[library] == "" {
		t.Fatal("universal shell library missing from pinned closure", r)
	}
	if err = os.WriteFile(library, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	if VerifyInputs(Lock{Runtimes: []Runtime{r}}, dir) == nil {
		t.Fatal("accepted upgraded fat shell library")
	}
}

func TestMachOPinsRelativeAndAbsoluteLibraryClosure(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "bin", "node")
	relative := filepath.Join(root, "lib", "libnode.dylib")
	absolute := filepath.Join(t.TempDir(), "libdependency.dylib")
	writeMachO(t, absolute, nil, nil)
	writeMachO(t, relative, []string{absolute}, nil)
	writeMachO(t, executable, []string{"@rpath/libnode.dylib", "/usr/lib/libSystem.B.dylib"}, []string{"@loader_path/../lib"})
	digest, _ := DigestFile(executable)
	r := Runtime{ID: "fixture", Command: []string{executable}, Files: map[string]string{executable: digest}}
	if err := pinNativeDependencies(&r); err != nil {
		t.Fatal(err)
	}
	if len(r.Files) != 2 || r.Files[relative] == "" || len(r.HostFiles) != 1 || r.HostFiles[absolute] == "" || r.NativeDependencyPolicy == "" {
		t.Fatal(r)
	}
	if err := VerifyInputs(Lock{Runtimes: []Runtime{r}}, root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte("upgraded"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyInputs(Lock{Runtimes: []Runtime{r}}, root); err == nil {
		t.Fatal("accepted changed actual runtime library")
	}
}

func TestMachORefusesUnresolvedLoaderRelativeLibrary(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "node")
	writeMachO(t, executable, []string{"@rpath/missing.dylib"}, []string{"@executable_path/../lib"})
	r := Runtime{Command: []string{executable}, Files: map[string]string{}}
	if err := pinNativeDependencies(&r); err == nil {
		t.Fatal("silently omitted required dependency")
	}
}

func TestMachOAbsoluteRpathIsHostPrerequisite(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "bin", "adapter")
	sdk := t.TempDir()
	library := filepath.Join(sdk, "libengine.dylib")
	writeMachO(t, library, nil, nil)
	writeMachO(t, exe, []string{"@rpath/libengine.dylib"}, []string{sdk})
	digest, _ := DigestFile(exe)
	r := Runtime{Command: []string{exe}, Files: map[string]string{exe: digest}}
	if err := pinNativeDependencies(&r); err != nil {
		t.Fatal(err)
	}
	if r.Files[library] != "" || r.HostFiles[library] == "" {
		t.Fatal("absolute SDK rpath incorrectly advertised as relocatable", r)
	}
}
