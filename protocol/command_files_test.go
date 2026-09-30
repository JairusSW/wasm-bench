package protocol

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCommandFileStreaming(t *testing.T) {
	dir := t.TempDir()
	data := bytes.Repeat([]byte("x"), CommandInputLimit+1)
	if err := os.WriteFile(filepath.Join(dir, "input"), data, 0600); err != nil {
		t.Fatal(err)
	}
	good := CommandFile{Path: "input", Size: uint64(len(data)), SHA256: CommandDigest(data)}
	var out bytes.Buffer
	if err := CopyCommandFile(&out, good, dir); err != nil || !bytes.Equal(out.Bytes(), data) {
		t.Fatal(err)
	}
	for _, mode := range []string{"size", "digest", "path", "mixed", "missing", "budget", "directory"} {
		t.Run(mode, func(t *testing.T) {
			bad := good
			switch mode {
			case "size":
				bad.Size--
			case "digest":
				bad.SHA256 = CommandDigest(nil)
			case "path":
				bad.Path = "../escape"
			case "mixed":
				bad.Data = []byte("x")
			case "missing":
				bad.Path = "missing"
			case "budget":
				bad.Size = CommandFileLimit + 1
			case "directory":
				bad.Path = dir
			}
			if CopyCommandFile(io.Discard, bad, dir) == nil {
				t.Fatal("accepted invalid file")
			}
		})
	}
}
