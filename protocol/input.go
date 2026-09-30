package protocol

import (
	"encoding/hex"
	"fmt"
)

func (input *MemoryInput) Apply(pointer func(string) ([]uint64, error), write func(uint32, []byte) bool) error {
	if input == nil {
		return nil
	}
	data, err := hex.DecodeString(input.Hex)
	if err != nil {
		return fmt.Errorf("invalid input hex: %w", err)
	}
	base := uint64(0)
	if input.PointerExport != "" {
		values, err := pointer(input.PointerExport)
		if err != nil {
			return fmt.Errorf("input pointer: %w", err)
		}
		if len(values) != 1 || values[0] > 0xffffffff {
			return fmt.Errorf("invalid wasm32 input pointer")
		}
		base = values[0]
	}
	offset := base + uint64(input.Offset)
	if offset > 0xffffffff || uint64(len(data)) > 0x100000000-offset {
		return fmt.Errorf("input address overflow")
	}
	if !write(uint32(offset), data) {
		return fmt.Errorf("input write outside guest memory")
	}
	return nil
}
