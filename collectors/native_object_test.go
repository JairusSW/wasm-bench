package collectors

import (
	"encoding/binary"
	"testing"
)

func machObjectFixture() []byte {
	// Mach-O 64 little endian with one regular instruction section and no symbols.
	b := make([]byte, 188)
	put := func(at int, value uint32) { binary.LittleEndian.PutUint32(b[at:], value) }
	put(0, 0xfeedfacf)
	put(4, 0x0100000c)
	put(12, 1)
	put(16, 1)
	put(20, 152)
	put(32, 0x19)
	put(36, 152)
	put(96, 1)
	copy(b[104:], "__text")
	copy(b[120:], "__TEXT")
	binary.LittleEndian.PutUint64(b[144:], 4)
	put(152, 184)
	put(168, 0x80000400)
	copy(b[184:], []byte{1, 2, 3, 4})
	return b
}
func TestNativeObjectSectionsMachO(t *testing.T) {
	b := machObjectFixture()
	sections, err := NativeObjectSections(b)
	if err != nil || len(sections) != 1 || sections[0].Name != "__TEXT,__text" || len(sections[0].Bytes) != 4 {
		t.Fatalf("sections=%v err=%v", sections, err)
	}
	// Debug/unwind sections are not executable, even when placed in __TEXT.
	binary.LittleEndian.PutUint32(b[168:], 0x6800000b)
	sections, err = NativeObjectSections(b)
	if err != nil || len(sections) != 0 {
		t.Fatalf("metadata included: %v %v", sections, err)
	}
}
func TestNativeObjectSectionsRejectIncomplete(t *testing.T) {
	b := machObjectFixture()
	for _, input := range [][]byte{nil, []byte("not an object"), b[:185]} {
		if _, err := NativeObjectSections(input); err == nil {
			t.Fatal("accepted malformed or truncated object")
		}
	}
	binary.LittleEndian.PutUint32(b[12:], 2)
	if _, err := NativeObjectSections(b); err == nil {
		t.Fatal("accepted executable instead of relocatable object")
	}
}

func TestNativeObjectSectionsELF(t *testing.T) {
	// ELF64 ET_REL: one executable PROGBITS section, one non-code section.
	b := make([]byte, 264)
	copy(b, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	binary.LittleEndian.PutUint16(b[16:], 1)
	binary.LittleEndian.PutUint16(b[18:], 62)
	binary.LittleEndian.PutUint32(b[20:], 1)
	binary.LittleEndian.PutUint64(b[40:], 72)
	binary.LittleEndian.PutUint16(b[52:], 64)
	binary.LittleEndian.PutUint16(b[58:], 64)
	binary.LittleEndian.PutUint16(b[60:], 3)
	copy(b[64:], []byte{1, 2, 3, 4, 5, 6, 7, 8})
	binary.LittleEndian.PutUint32(b[140:], 1) // section 1 PROGBITS
	binary.LittleEndian.PutUint64(b[144:], 6) // ALLOC | EXECINSTR
	binary.LittleEndian.PutUint64(b[160:], 64)
	binary.LittleEndian.PutUint64(b[168:], 4)
	binary.LittleEndian.PutUint32(b[204:], 1) // section 2 data
	binary.LittleEndian.PutUint64(b[224:], 68)
	binary.LittleEndian.PutUint64(b[232:], 4)
	sections, err := NativeObjectSections(b)
	if err != nil || len(sections) != 1 || len(sections[0].Bytes) != 4 {
		t.Fatalf("sections=%v err=%v", sections, err)
	}
	binary.LittleEndian.PutUint64(b[168:], 1000)
	if _, err := NativeObjectSections(b); err == nil {
		t.Fatal("accepted truncated ELF section")
	}
	binary.LittleEndian.PutUint64(b[168:], 4)
	binary.LittleEndian.PutUint16(b[16:], 2)
	if _, err := NativeObjectSections(b); err == nil {
		t.Fatal("accepted linked ELF executable")
	}
}
