package main

import (
	"bytes"
	"encoding/binary"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"hash/crc32"
	"testing"
)

func TestWazevoNativeImageKeepsTimingCacheDisabled(t *testing.T) {
	workloads, err := corpus.Generate(t.TempDir(), "core")
	if err != nil {
		t.Fatal(err)
	}
	a := &adapter{}
	defer a.close()
	w := workloads[0]
	if err := a.prepare(&protocol.Preparation{Artifact: w.Artifact, ArtifactSHA256: w.SHA256, Workload: w, Profile: "code"}); err != nil {
		t.Fatal(err)
	}
	image, observations, err := a.inspectCode()
	if err != nil {
		t.Fatal(err)
	}
	if image == nil || len(image.Data) == 0 || image.Backend != "wazevo" || len(observations) != 2 || observations[1].Metric != "native.code_size" || *observations[1].Value != float64(len(image.Data)) || *observations[0].Value != float64(len(image.Data)) {
		t.Fatalf("missing native segment: %+v", image)
	}
	if a.engine != nil || a.compiled != nil {
		t.Fatal("code export seeded the timing runtime")
	}
	other, _, err := a.inspectCode()
	if err != nil {
		t.Fatal(err)
	}
	if other.SHA256 != image.SHA256 {
		t.Fatal("native image changed on identical source")
	}
	a.interpreter = true
	image, observations, err = a.inspectCode()
	if err != nil || image != nil || observations[0].Status != "not_applicable" {
		t.Fatal("interpreter claimed native code")
	}
}
func TestWazevoCacheParserRejectsTruncationAndCorruption(t *testing.T) {
	var b bytes.Buffer
	b.WriteString("WAZEVO")
	b.WriteByte(7)
	b.WriteString("v1.12.0")
	binary.Write(&b, binary.LittleEndian, uint32(1))
	binary.Write(&b, binary.LittleEndian, uint64(0))
	binary.Write(&b, binary.LittleEndian, uint64(4))
	b.Write([]byte{1, 2, 3, 4})
	binary.Write(&b, binary.LittleEndian, crc32.Checksum([]byte{1, 2, 3, 4}, crc32.MakeTable(crc32.Castagnoli)))
	data := b.Bytes()
	code, size, err := readWazevoSegment(bytes.NewReader(data))
	if err != nil || size != 4 || !bytes.Equal(code, []byte{1, 2, 3, 4}) {
		t.Fatal("valid segment rejected", err)
	}
	for i := 0; i < len(data); i++ {
		if _, _, err := readWazevoSegment(bytes.NewReader(data[:i])); err == nil {
			t.Fatalf("accepted truncation at %d", i)
		}
	}
	changed := bytes.Clone(data)
	changed[len(changed)-5] ^= 1
	if _, _, err := readWazevoSegment(bytes.NewReader(changed)); err == nil {
		t.Fatal("accepted changed native bytes")
	}
	changed = bytes.Clone(data)
	changed[7] = 'x'
	if _, _, err := readWazevoSegment(bytes.NewReader(changed)); err == nil {
		t.Fatal("accepted unknown cache format version")
	}
}
