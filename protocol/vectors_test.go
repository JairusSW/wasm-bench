package protocol

import (
	"errors"
	"slices"
	"testing"
)

func TestVectorPreparationRejectsInvalidContract(t *testing.T) {
	for _, v := range []VectorContract{
		{}, {OutputLen: 1, Cases: []VectorCase{{Len: -1, Out: "ab"}}},
		{OutputLen: 1, Cases: []VectorCase{{Out: "abc"}}},
		{OutputLen: 1, Cases: []VectorCase{{Out: "zz"}}},
		{OutputLen: 2, Cases: []VectorCase{{Out: "ab"}}},
		{OutputLen: 1, Mod: -1, Cases: []VectorCase{{Out: "ab"}}},
		{OutputLen: 1, Cases: []VectorCase{{Len: 100, Out: "ab"}}},
	} {
		if _, err := PrepareVectors(v, 10); err == nil {
			t.Fatalf("accepted %+v", v)
		}
	}
}

func TestVectorsOrderedAndRepeated(t *testing.T) {
	v := VectorContract{InputOffset: 999, OutputOffset: 999, InputPointerExport: "in", OutputPointerExport: "out", OutputLen: 1, Mod: 3, Cases: []VectorCase{{Len: 0, Out: "00"}, {Len: 4, Out: "03"}, {Len: 5, Out: "04"}}}
	p, err := PrepareVectors(v, 12)
	if err != nil {
		t.Fatal(err)
	}
	v.Cases[0].Out = "ff" // prepared data is owned, not caller-mutable.
	for repeat := 0; repeat < 2; repeat++ {
		memory := make([]byte, 64)
		var order []uint32
		pointers := 0
		err = p.Run(VectorInstance{
			Pointer: func(name string) ([]uint64, error) {
				pointers++
				if name == "in" {
					return []uint64{4}, nil
				}
				return []uint64{32}, nil
			},
			Write: func(offset uint32, b []byte) bool { copy(memory[offset:], b); return true },
			Invoke: func(in, n, out uint32) error {
				order = append(order, n)
				var sum byte
				for j, b := range memory[in : in+n] {
					if b != byte(j%3) {
						t.Fatal("wrong input pattern")
					}
					sum += b
				}
				memory[out] = sum
				return nil
			},
			Read: func(offset, n uint32) ([]byte, bool) { return memory[offset : offset+n], true },
		})
		if err != nil || pointers != 2 || !slices.Equal(order, []uint32{0, 4, 5}) {
			t.Fatal(err, pointers, order)
		}
	}
}

func TestVectorsFailClosed(t *testing.T) {
	if err := (&PreparedVectors{}).Run(VectorInstance{}); err == nil {
		t.Fatal("accepted unprepared vectors")
	}
	for _, mode := range []string{"pointer", "overflow", "write", "invoke", "read", "mismatch"} {
		t.Run(mode, func(t *testing.T) {
			v := VectorContract{InputPointerExport: "in", OutputLen: 1, Cases: []VectorCase{{Len: 2, Out: "ab"}}}
			p, err := PrepareVectors(v, 3)
			if err != nil {
				t.Fatal(err)
			}
			err = p.Run(VectorInstance{
				Pointer: func(string) ([]uint64, error) {
					if mode == "pointer" {
						return nil, nil
					}
					if mode == "overflow" {
						return []uint64{0xffffffff}, nil
					}
					return []uint64{0}, nil
				},
				Write: func(uint32, []byte) bool { return mode != "write" },
				Invoke: func(uint32, uint32, uint32) error {
					if mode == "invoke" {
						return errors.New("trap")
					}
					return nil
				},
				Read: func(uint32, uint32) ([]byte, bool) {
					if mode == "mismatch" {
						return []byte{0}, true
					}
					return []byte{0xab}, mode != "read"
				},
			})
			if err == nil {
				t.Fatal("accepted invalid vector execution")
			}
		})
	}
}
