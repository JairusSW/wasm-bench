package experiment

import (
	"encoding/binary"
	"os"
	"path/filepath"
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
