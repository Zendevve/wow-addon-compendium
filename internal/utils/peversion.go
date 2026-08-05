package utils

import (
	"encoding/binary"
	"fmt"
	"os"
)

// FileVersion extracts the version resource from a Windows PE
// executable. It works on any host OS because it parses the file
// format directly. Returns ok=false when the file has no version info.
func FileVersion(path string) (version string, ok bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}

	if len(data) < 64 || string(data[0:2]) != "MZ" {
		return "", false, fmt.Errorf("%s: not a PE executable", path)
	}

	peOff := int(binary.LittleEndian.Uint32(data[0x3C:]))
	if peOff+24 > len(data) || string(data[peOff:peOff+4]) != "PE\x00\x00" {
		return "", false, fmt.Errorf("%s: corrupt PE header", path)
	}

	// COFF header (20 bytes) precedes the optional header.
	optOff := peOff + 24
	if optOff+2 > len(data) {
		return "", false, fmt.Errorf("%s: truncated optional header", path)
	}
	magic := binary.LittleEndian.Uint16(data[optOff:])
	var dirOffset int
	switch magic {
	case 0x10B: // PE32
		dirOffset = optOff + 96
	case 0x20B: // PE32+
		dirOffset = optOff + 112
	default:
		return "", false, fmt.Errorf("%s: unknown optional header magic", path)
	}

	numDirs := int(binary.LittleEndian.Uint32(data[optOff+92:]))
	if magic == 0x20B {
		numDirs = int(binary.LittleEndian.Uint32(data[optOff+108:]))
	}
	if numDirs <= 2 {
		return "", false, nil
	}

	// Data directory index 2 = resource directory.
	resRVA := binary.LittleEndian.Uint32(data[dirOffset+2*8:])
	resSize := binary.LittleEndian.Uint32(data[dirOffset+2*8+4:])
	if resRVA == 0 || resSize == 0 {
		return "", false, nil
	}

	// Section headers follow the optional header.
	secSize := int(binary.LittleEndian.Uint16(data[optOff+60:])) // SizeOfOptionalHeader
	secOff := optOff + secSize
	numSec := int(binary.LittleEndian.Uint16(data[optOff+6:]))

	rvaToOffset := func(rva uint32) (uint32, bool) {
		for i := 0; i < numSec; i++ {
			s := secOff + i*40
			if s+40 > len(data) {
				return 0, false
			}
			vAddr := binary.LittleEndian.Uint32(data[s+12:])
			vSize := binary.LittleEndian.Uint32(data[s+8:])
			rawSize := binary.LittleEndian.Uint32(data[s+16:])
			rawPtr := binary.LittleEndian.Uint32(data[s+20:])
			size := vSize
			if rawSize > size {
				size = rawSize
			}
			if rva >= vAddr && rva < vAddr+size {
				return rawPtr + (rva - vAddr), true
			}
		}
		return 0, false
	}

	type resourceEntry struct {
		id    uint32
		offTo uint32
	}

	// Walk the resource tree: type(16=RT_VERSION) -> id -> language -> data.
	dirAt := func(rva uint32) ([]resourceEntry, bool) {
		off, ok := rvaToOffset(rva)
		if !ok || int(off)+16 > len(data) {
			return nil, false
		}
		named := int(binary.LittleEndian.Uint16(data[off+12:]))
		id := int(binary.LittleEndian.Uint16(data[off+14:]))
		total := named + id
		entries := make([]resourceEntry, 0, total)
		base := int(off) + 16
		for i := 0; i < total; i++ {
			e := base + i*8
			if e+8 > len(data) {
				return nil, false
			}
			entries = append(entries, resourceEntry{
				id:    binary.LittleEndian.Uint32(data[e:]),
				offTo: binary.LittleEndian.Uint32(data[e+4:]),
			})
		}
		return entries, true
	}

	// Level 1: find the RT_VERSION (16) type entry.
	level1, ok := dirAt(resRVA)
	if !ok {
		return "", false, nil
	}
	var versionDir uint32
	found := false
	for _, e := range level1 {
		if e.id == 16 && e.offTo&0x80000000 != 0 {
			versionDir = e.offTo &^ 0x80000000
			found = true
			break
		}
	}
	if !found {
		return "", false, nil
	}

	// Level 2: any id entry -> subdirectory.
	level2, ok := dirAt(versionDir)
	if !ok || len(level2) == 0 {
		return "", false, nil
	}
	var langDir uint32
	found = false
	for _, e := range level2 {
		if e.offTo&0x80000000 != 0 {
			langDir = e.offTo &^ 0x80000000
			found = true
			break
		}
	}
	if !found {
		return "", false, nil
	}

	// Level 3: any language entry -> data entry.
	level3, ok := dirAt(langDir)
	if !ok || len(level3) == 0 {
		return "", false, nil
	}
	var dataRVA, dataSize uint32
	found = false
	for _, e := range level3 {
		if e.offTo&0x80000000 == 0 {
			dataRVA = e.offTo
			dataSize = 0 // size read from the data entry below
			off, ok := rvaToOffset(dataRVA)
			if ok && int(off)+8 <= len(data) {
				dataSize = binary.LittleEndian.Uint32(data[off+4:])
			}
			found = true
			break
		}
	}
	if !found {
		return "", false, nil
	}

	off, ok := rvaToOffset(dataRVA)
	if !ok || int(off)+int(dataSize) > len(data) {
		return "", false, nil
	}
	verData := data[off : off+dataSize]

	// VS_VERSIONINFO: wLength(2) wValueLength(2) wType(2) szKey(16)
	// then VS_FIXEDFILEINFO at the next 4-byte boundary.
	valueLen := int(binary.LittleEndian.Uint16(verData[2:]))
	if valueLen < 52 || len(verData) < 24 {
		return "", false, nil
	}
	ffi := 24 // 6 + 16 key, aligned to 4
	if ffi+52 > len(verData) {
		return "", false, nil
	}
	if binary.LittleEndian.Uint32(verData[ffi:]) != 0xFEEF04BD {
		return "", false, fmt.Errorf("%s: bad VS_FIXEDFILEINFO signature", path)
	}
	ms := binary.LittleEndian.Uint32(verData[ffi+8:])
	ls := binary.LittleEndian.Uint32(verData[ffi+12:])
	version = fmt.Sprintf("%d.%d.%d.%d",
		ms>>16, ms&0xFFFF, ls>>16, ls&0xFFFF)
	return version, true, nil
}
