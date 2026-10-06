package publish

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSiteAnalyticalFilesPreserveSealedBytes(t *testing.T) {
	dir := t.TempDir()
	content := append(bytes.Repeat([]byte{0, 255, 17}, SiteFileChunkBytes/3+1), []byte("original parquet tail")...)
	if err := os.WriteFile(filepath.Join(dir, "samples.parquet"), content, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "memory-samples.parquet"), []byte("unsealed extra"), 0600); err != nil {
		t.Fatal(err)
	}
	seal, _ := json.Marshal(map[string]string{"samples.parquet": siteHash(content)})
	chunks := map[string][]byte{}
	var descriptor SiteReportFile
	binary := func(b []byte) (string, error) { id := siteHash(b); chunks[id] = append([]byte{}, b...); return id, nil }
	object := func(kind string, v any) (string, error) {
		r := v.(SiteRecord)
		if kind != "record" || r.Kind != "report-file" || siteHash(r.Data) != r.ID {
			t.Fatal("invalid file record")
		}
		if err := json.Unmarshal(r.Data, &descriptor); err != nil {
			t.Fatal(err)
		}
		return siteID(v)
	}
	files, err := siteReportFiles(dir, seal, siteHash([]byte("report")), binary, object)
	if err != nil || len(files) != 1 || len(descriptor.Chunks) != 2 || descriptor.Bytes != int64(len(content)) {
		t.Fatal(files, descriptor, err)
	}
	var assembled []byte
	for _, chunk := range descriptor.Chunks {
		b := chunks[chunk.SHA256]
		if len(b) != chunk.Bytes {
			t.Fatal("wrong chunk size")
		}
		assembled = append(assembled, b...)
	}
	if !bytes.Equal(assembled, content) || siteHash(assembled) != descriptor.SHA256 {
		t.Fatal("changed analytical bytes")
	}
	if err := os.WriteFile(filepath.Join(dir, "samples.parquet"), []byte("changed after verification"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := siteReportFiles(dir, seal, descriptor.ReportID, binary, object); err == nil {
		t.Fatal("changed sealed file accepted")
	}
	if err := os.Remove(filepath.Join(dir, "samples.parquet")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "memory-samples.parquet"), filepath.Join(dir, "samples.parquet")); err != nil {
		t.Fatal(err)
	}
	if _, err := siteReportFiles(dir, seal, descriptor.ReportID, binary, object); err == nil {
		t.Fatal("symlink accepted")
	}
}
