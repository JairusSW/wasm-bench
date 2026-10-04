package collectors

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"fmt"
)

// ObjectCodeSection is executable content in a relocatable compiler object.
// Bytes have not been linked: relocations remain in the original object. These
// sections must never be described as a live executable image.
type ObjectCodeSection struct {
	Name  string
	Bytes []byte
}

// NativeObjectSections excludes symbols, debug data, unwind tables and object
// headers. Only relocatable ELF and Mach-O objects are accepted. A valid module
// with no defined functions may produce no executable sections.
func NativeObjectSections(object []byte) ([]ObjectCodeSection, error) {
	if len(object) >= 4 && bytes.Equal(object[:4], []byte{0x7f, 'E', 'L', 'F'}) {
		file, err := elf.NewFile(bytes.NewReader(object))
		if err != nil {
			return nil, err
		}
		defer file.Close()
		if file.Type != elf.ET_REL {
			return nil, fmt.Errorf("native object: ELF is not relocatable")
		}
		var result []ObjectCodeSection
		for _, section := range file.Sections {
			if section.Flags&elf.SHF_EXECINSTR == 0 {
				continue
			}
			if section.Flags&elf.SHF_COMPRESSED != 0 {
				return nil, fmt.Errorf("native object: compressed executable section %q is unsupported", section.Name)
			}
			if section.Type != elf.SHT_PROGBITS {
				return nil, fmt.Errorf("native object: executable section %q has no file-backed code", section.Name)
			}
			if section.Offset > uint64(len(object)) || section.Size > uint64(len(object))-section.Offset {
				return nil, fmt.Errorf("native object: truncated section %q", section.Name)
			}
			data, err := section.Data()
			if err != nil {
				return nil, err
			}
			result = append(result, ObjectCodeSection{section.Name, data})
		}
		return result, nil
	}
	file, err := macho.NewFile(bytes.NewReader(object))
	if err != nil {
		return nil, fmt.Errorf("native object: unsupported or malformed object: %w", err)
	}
	defer file.Close()
	if file.Type != macho.TypeObj {
		return nil, fmt.Errorf("native object: Mach-O is not relocatable")
	}
	var result []ObjectCodeSection
	for _, section := range file.Sections {
		// S_ATTR_PURE_INSTRUCTIONS | S_ATTR_SOME_INSTRUCTIONS, from loader.h.
		if section.Flags&0x80000400 == 0 {
			continue
		}
		if section.Flags&0xff != 0 {
			return nil, fmt.Errorf("native object: executable section %q is not regular file-backed code", section.Name)
		}
		if uint64(section.Offset) > uint64(len(object)) || section.Size > uint64(len(object))-uint64(section.Offset) {
			return nil, fmt.Errorf("native object: truncated section %q", section.Name)
		}
		data, err := section.Data()
		if err != nil {
			return nil, err
		}
		result = append(result, ObjectCodeSection{section.Seg + "," + section.Name, data})
	}
	return result, nil
}
