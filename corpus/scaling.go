package corpus

import "fmt"

var ScalingDimensions = []string{"functions", "body", "locals", "nesting", "branch-table", "types", "data-segments", "imports"}

// ScalingModule changes one declared structural dimension. Types are distinct
// fixed-arity signatures; imports are repeated bindings of a versioned identity
// host function. All modules have an executable, deterministic result oracle.
func ScalingModule(dimension string, size int) ([]byte, error) {
	if size < 1 || size > 65536 {
		return nil, fmt.Errorf("scaling size must be in [1,65536]")
	}
	if dimension == "functions" || dimension == "body" {
		return Module(dimension, size), nil
	}
	b := []byte{0, 97, 115, 109, 1, 0, 0, 0}
	types := []byte{1, 0x60, 1, 0x7f, 1, 0x7f}
	if dimension == "types" {
		types = append(uleb(size+1), 0x60, 1, 0x7f, 1, 0x7f)
		for i := 0; i < size; i++ {
			types = append(types, 0x60, 8)
			n := i
			for j := 0; j < 8; j++ {
				types = append(types, byte(0x7f-(n&3)))
				n >>= 2
			}
			types = append(types, 0)
		}
	}
	b = append(b, section(1, types)...)
	imports := 0
	if dimension == "imports" {
		imports = size
		p := uleb(size)
		for i := 0; i < size; i++ {
			p = append(p, 9, 'w', 'a', 's', 'm', 'b', 'e', 'n', 'c', 'h', 8, 'i', 'd', 'e', 'n', 't', 'i', 't', 'y', 0, 0)
		}
		b = append(b, section(2, p)...)
	}
	b = append(b, section(3, []byte{1, 0})...)
	b = append(b, section(5, []byte{1, 0, 1})...)
	exports := []byte{2, 9, 'b', 'e', 'n', 'c', 'h', 'm', 'a', 'r', 'k', 0}
	exports = append(exports, uleb(imports)...)
	exports = append(exports, 6, 'm', 'e', 'm', 'o', 'r', 'y', 2, 0)
	b = append(b, section(7, exports)...)
	body := []byte{0, 0x20, 0, 0x0b}
	switch dimension {
	case "locals":
		body = append([]byte{1}, uleb(size)...)
		body = append(body, 0x7f, 0x20, 0, 0x21)
		body = append(body, uleb(size)...)
		body = append(body, 0x20)
		body = append(body, uleb(size)...)
		body = append(body, 0x0b)
	case "nesting":
		body = []byte{0}
		for i := 0; i < size; i++ {
			body = append(body, 0x02, 0x7f)
		}
		body = append(body, 0x20, 0)
		for i := 0; i < size; i++ {
			body = append(body, 0x0b)
		}
		body = append(body, 0x0b)
	case "branch-table":
		body = []byte{0, 0x02, 0x40, 0x20, 0, 0x0e}
		body = append(body, uleb(size)...)
		for i := 0; i <= size; i++ {
			body = append(body, 0)
		}
		body = append(body, 0x0b, 0x20, 0, 0x0b)
	case "types":
	case "data-segments":
		body = []byte{0, 0x20, 0, 0x41, 0, 0x2d, 0, 0, 0x6a, 0x0b}
	case "imports":
		body = []byte{0, 0x20, 0, 0x10}
		body = append(body, uleb(size-1)...)
		body = append(body, 0x0b)
	default:
		return nil, fmt.Errorf("unknown scaling dimension %q", dimension)
	}
	code := append([]byte{1}, uleb(len(body))...)
	code = append(code, body...)
	b = append(b, section(10, code)...)
	if dimension == "data-segments" {
		data := uleb(size)
		for i := 0; i < size; i++ {
			data = append(data, 0, 0x41, 0, 0x0b, 1, byte(i))
		}
		b = append(b, section(11, data)...)
	}
	return b, nil
}
