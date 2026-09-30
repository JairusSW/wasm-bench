package protocol

import (
	"errors"
	"testing"
)

func TestMemoryInput(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  *MemoryInput
		values []uint64
		err    error
		fail   bool
		offset uint32
	}{
		{name: "nil"},
		{name: "absolute", input: &MemoryInput{Offset: 7, Hex: "abcd"}, offset: 7},
		{name: "relative", input: &MemoryInput{PointerExport: "ptr", Offset: 7, Hex: "abcd"}, values: []uint64{16}, offset: 23},
		{name: "bad hex", input: &MemoryInput{Hex: "a"}, fail: true},
		{name: "bad arity", input: &MemoryInput{PointerExport: "ptr"}, values: []uint64{1, 2}, fail: true},
		{name: "large pointer", input: &MemoryInput{PointerExport: "ptr"}, values: []uint64{1 << 32}, fail: true},
		{name: "offset overflow", input: &MemoryInput{PointerExport: "ptr", Offset: 1}, values: []uint64{0xffffffff}, fail: true},
		{name: "length overflow", input: &MemoryInput{Offset: 0xffffffff, Hex: "abcd"}, fail: true},
		{name: "pointer error", input: &MemoryInput{PointerExport: "ptr"}, err: errors.New("trap"), fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			err := tc.input.Apply(func(string) ([]uint64, error) { return tc.values, tc.err }, func(offset uint32, data []byte) bool {
				writes++
				if offset != tc.offset {
					t.Fatalf("offset %d != %d", offset, tc.offset)
				}
				return true
			})
			if (err != nil) != tc.fail {
				t.Fatalf("unexpected error %v", err)
			}
			if (tc.fail || tc.input == nil) && writes != 0 {
				t.Fatal("invalid input wrote memory")
			}
			if !tc.fail && tc.input != nil && writes != 1 {
				t.Fatal("missing write")
			}
		})
	}
	if err := (&MemoryInput{Hex: "ab"}).Apply(nil, func(uint32, []byte) bool { return false }); err == nil {
		t.Fatal("ignored failed write")
	}
}
