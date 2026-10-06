package publish

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestSiteReportArchivePreservesSealedFilesAndIsReproducible(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "raw"), 0700); err != nil {
		t.Fatal(err)
	}
	content := bytes.Repeat([]byte("original trial bytes\n"), 1000)
	if err := os.WriteFile(filepath.Join(dir, "raw", "trial.json"), content, 0600); err != nil {
		t.Fatal(err)
	}
	seal, _ := json.Marshal(map[string]string{"raw/trial.json": siteHash(content)})
	if err := os.WriteFile(filepath.Join(dir, "checksums.json"), seal, 0600); err != nil {
		t.Fatal(err)
	}
	export := func() ([]byte, SiteReportFile, error) {
		objects := map[string][]byte{}
		var descriptor SiteReportFile
		err := siteReportArchive(dir, seal, siteHash([]byte("report")), func(b []byte) (string, error) {
			id := siteHash(b)
			objects[id] = append([]byte{}, b...)
			return id, nil
		}, func(kind string, v any) (string, error) {
			r := v.(SiteRecord)
			if e := json.Unmarshal(r.Data, &descriptor); e != nil {
				return "", e
			}
			return r.ID, nil
		})
		var body []byte
		for _, c := range descriptor.Chunks {
			body = append(body, objects[c.SHA256]...)
		}
		return body, descriptor, err
	}
	first, descriptor, err := export()
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := export()
	if err != nil || !bytes.Equal(first, second) {
		t.Fatal("archive representation drift", err)
	}
	if descriptor.PackingVersion != SiteArchivePacking || descriptor.SourceSealSHA256 != siteHash(seal) || descriptor.SHA256 != siteHash(first) || descriptor.Bytes != int64(len(first)) {
		t.Fatal(descriptor)
	}
	gz, err := gzip.NewReader(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	files := map[string][]byte{}
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		b, e := io.ReadAll(tr)
		if e != nil {
			t.Fatal(e)
		}
		files[h.Name] = b
	}
	if len(files) != 2 || !bytes.Equal(files["checksums.json"], seal) || !bytes.Equal(files["raw/trial.json"], content) {
		t.Fatal("archive changed source files")
	}
	if err := os.WriteFile(filepath.Join(dir, "raw", "trial.json"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = export(); err == nil {
		t.Fatal("changed source archived")
	}
	if err := os.Remove(filepath.Join(dir, "raw", "trial.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "checksums.json"), filepath.Join(dir, "raw", "trial.json")); err != nil {
		t.Fatal(err)
	}
	if _, _, err = export(); err == nil {
		t.Fatal("symlink archived")
	}
}
func TestSiteReportArchiveRejectsEscapingSealPaths(t *testing.T) {
	for _, name := range []string{"../outside", "/absolute", "raw/../outside", "raw\\outside", "checksums.json"} {
		seal, _ := json.Marshal(map[string]string{name: siteHash(nil)})
		err := siteReportArchive(t.TempDir(), seal, siteHash(nil), func(b []byte) (string, error) { return siteHash(b), nil }, func(string, any) (string, error) { return "", nil })
		if err == nil {
			t.Fatal("unsafe archive path admitted", name)
		}
	}
}
