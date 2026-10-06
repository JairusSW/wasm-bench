package publish

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const SiteArchivePacking = "sealed-files-tar-gzip-v1"
const siteArchiveRawBytes int64 = 8 << 30

// This is an offline derivative of exact sealed files, not a reconstructed
// report or a hermetic tool bundle. Neither analysis nor archived code runs.
func siteReportArchive(report string, seal []byte, reportID string, binary func([]byte) (string, error), object func(string, any) (string, error)) error {
	if report == "" {
		return nil
	}
	if len(seal) > 16<<20 {
		return fmt.Errorf("report archive seal exceeds ceiling")
	}
	var hashes map[string]string
	if err := json.Unmarshal(seal, &hashes); err != nil {
		return err
	}
	if len(hashes) == 0 || len(hashes) > 100000 {
		return fmt.Errorf("report archive file count exceeds ceiling")
	}
	info, err := os.Lstat(report)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("report archive root is not a regular directory")
	}
	root, err := os.OpenRoot(report)
	if err != nil {
		return err
	}
	defer root.Close()
	names := []string{}
	for name, digest := range hashes {
		if !fs.ValidPath(name) || name == "checksums.json" || strings.ContainsAny(name, "\\\x00") || len(name) > 4096 || len(digest) != 64 {
			return fmt.Errorf("invalid sealed archive path or digest")
		}
		if _, err := hex.DecodeString(digest); err != nil {
			return fmt.Errorf("invalid sealed archive digest")
		}
		names = append(names, name)
	}
	sort.Strings(names)
	writer := &siteArchiveWriter{binary: binary, hash: sha256.New(), buffer: make([]byte, 0, SiteFileChunkBytes)}
	gz, err := gzip.NewWriterLevel(writer, gzip.BestSpeed)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(gz)
	// Fixed ordering, permissions, ownership and times make the representation
	// reproducible while retaining every original file byte and the exact seal.
	header := func(name string, size int64) error {
		return tw.WriteHeader(&tar.Header{Name: name, Size: size, Mode: 0644, ModTime: time.Unix(0, 0), Typeflag: tar.TypeReg})
	}
	sealFile, err := archiveRegular(root, "checksums.json")
	if err != nil {
		return err
	}
	actual, err := io.ReadAll(io.LimitReader(sealFile, int64(len(seal))+1))
	sealFile.Close()
	if err != nil {
		return err
	}
	if !bytes.Equal(actual, seal) {
		return fmt.Errorf("report archive seal changed")
	}
	if err = header("checksums.json", int64(len(seal))); err != nil {
		return err
	}
	if _, err = tw.Write(seal); err != nil {
		return err
	}
	rawBytes := int64(len(seal))
	for _, name := range names {
		file, err := archiveRegular(root, name)
		if err != nil {
			return err
		}
		stat, err := file.Stat()
		if err != nil {
			file.Close()
			return err
		}
		rawBytes += stat.Size()
		if stat.Size() < 0 || rawBytes > siteArchiveRawBytes {
			file.Close()
			return fmt.Errorf("report archive raw bytes exceed ceiling")
		}
		if err = header(name, stat.Size()); err != nil {
			file.Close()
			return err
		}
		digest := sha256.New()
		_, err = io.CopyN(io.MultiWriter(tw, digest), file, stat.Size())
		if err == nil {
			var extra [1]byte
			n, e := file.Read(extra[:])
			if n != 0 || e != io.EOF {
				err = fmt.Errorf("sealed archive file grew during export")
			}
		}
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if hex.EncodeToString(digest.Sum(nil)) != hashes[name] {
			return fmt.Errorf("sealed archive file changed during export")
		}
	}
	if err = tw.Close(); err != nil {
		return err
	}
	if err = gz.Close(); err != nil {
		return err
	}
	if err = writer.flush(); err != nil {
		return err
	}
	descriptor := SiteReportFile{Schema: 1, Kind: "report-file", ReportID: reportID, Name: "report.tar.gz", MediaType: "application/gzip", Encoding: "identity", SHA256: hex.EncodeToString(writer.hash.Sum(nil)), Bytes: writer.bytes, Chunks: writer.chunks, PackingVersion: SiteArchivePacking, SourceSealSHA256: siteHash(seal)}
	data, err := siteJSON(descriptor)
	if err != nil {
		return err
	}
	_, err = object("record", SiteRecord{Kind: "report-file", ID: siteHash(data), Data: data})
	return err
}

func archiveRegular(root *os.Root, name string) (*os.File, error) {
	parts := strings.Split(name, "/")
	for i := range parts {
		info, err := root.Lstat(filepath.Join(parts[:i+1]...))
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || i < len(parts)-1 && !info.IsDir() || i == len(parts)-1 && !info.Mode().IsRegular() {
			return nil, fmt.Errorf("sealed archive path is not regular")
		}
	}
	before, err := root.Lstat(filepath.FromSlash(name))
	if err != nil {
		return nil, err
	}
	file, err := root.Open(filepath.FromSlash(name))
	if err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) {
		file.Close()
		return nil, fmt.Errorf("sealed archive file changed during open")
	}
	return file, nil
}

type siteArchiveWriter struct {
	binary func([]byte) (string, error)
	hash   hash.Hash
	buffer []byte
	chunks []SiteFileChunk
	bytes  int64
}

func (w *siteArchiveWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > SiteFileBytes-w.bytes {
		return 0, fmt.Errorf("compressed report archive exceeds ceiling")
	}
	original := len(p)
	w.bytes += int64(original)
	w.hash.Write(p)
	for len(p) > 0 {
		n := SiteFileChunkBytes - len(w.buffer)
		if n > len(p) {
			n = len(p)
		}
		w.buffer = append(w.buffer, p[:n]...)
		p = p[n:]
		if len(w.buffer) == SiteFileChunkBytes {
			if err := w.flush(); err != nil {
				return original - len(p), err
			}
		}
	}
	return original, nil
}
func (w *siteArchiveWriter) flush() error {
	if len(w.buffer) == 0 {
		return nil
	}
	digest, err := w.binary(w.buffer)
	if err != nil {
		return err
	}
	w.chunks = append(w.chunks, SiteFileChunk{digest, len(w.buffer)})
	w.buffer = w.buffer[:0]
	return nil
}
