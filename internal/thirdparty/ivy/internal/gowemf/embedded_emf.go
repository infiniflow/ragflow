// Copied from github.com/mgilbir/gowemf, file embedded_emf.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

// ExtractEnhancedMetafile assembles a single enhanced metafile embedded in WMFC
// META_ESCAPE records. It returns nil,nil when there is no such stream. Returned
// bytes are independently owned. Both outer and inner framing, fragment counts,
// remaining sizes and the one's-complement XOR checksum are validated. A checksum
// failure does not invalidate the independently playable WMF fallback; callers
// can still use Stream on the original WMF. No implicit recursive playback occurs.
func ExtractEnhancedMetafile(data []byte, limits Limits) ([]byte, error) {
	l := limits.defaults()
	header, err := Walk(data, l, nil)
	if err != nil {
		return nil, err
	}
	if header.Format != WMF {
		return nil, failure(0, "enhanced metafile requires WMF container", ErrFormat)
	}
	var result []byte
	var total, records, seen uint32
	var checksum uint16
	started, complete := false, false
	_, err = Walk(data, l, func(r Record) error {
		if r.Type&255 != MetaEscape&255 || len(r.Data) < 2 || u16(r.Data) != 15 {
			return nil
		}
		v, err := Decode(r, DecodeLimits{MaxObjectBytes: l.MaxBytes, MaxElements: l.MaxRecords})
		if err != nil {
			return err
		}
		f, ok := v.(WMFEnhancedMetafile)
		if !ok { // a private comment
			return nil
		}
		if complete {
			return malformed(r.Offset, "multiple embedded EMF streams")
		}
		if !started {
			started = true
			total, records, checksum = f.TotalBytes, f.Records, f.Checksum
			if uint64(total) > l.MaxBytes || uint64(total) > uint64(int(^uint(0)>>1)) || uint64(records) > l.MaxRecords {
				return failure(r.Offset, "embedded EMF budget", ErrLimit)
			}
		}
		if f.TotalBytes != total || f.Records != records || seen >= records {
			return malformed(r.Offset, "inconsistent enhanced-metafile fragments")
		}
		// Producers can leave checksums in continuation envelopes zero. A
		// nonzero continuation checksum must agree; the complete checksum is
		// always checked against the first envelope after assembly.
		if seen != 0 && f.Checksum != 0 && f.Checksum != checksum {
			return malformed(r.Offset, "inconsistent embedded checksum")
		}
		n := uint64(len(result)) + uint64(len(f.Data))
		if n > uint64(total) || uint64(f.Remaining) != uint64(total)-n {
			return malformed(r.Offset, "embedded EMF remaining bytes")
		}
		if n > uint64(cap(result)) {
			capacity := uint64(cap(result)) * 2
			if capacity < n {
				capacity = n
			}
			if capacity > uint64(total) {
				capacity = uint64(total)
			}
			grown := make([]byte, len(result), int(capacity))
			copy(grown, result)
			result = grown
		}
		result = append(result, f.Data...)
		seen++
		if f.Remaining == 0 {
			if seen != records {
				return malformed(r.Offset, "embedded EMF record count")
			}
			complete = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !started {
		return nil, nil
	}
	if !complete {
		return nil, malformed(len(data), "unfinished embedded EMF")
	}
	inner, err := Walk(result, l, nil)
	if err != nil {
		return nil, err
	}
	if inner.Format != EMF {
		return nil, malformed(0, "embedded data is not EMF")
	}
	var sum uint16
	for off := 0; off < len(result); off += 2 {
		sum ^= u16(result[off:])
	}
	if ^sum != checksum {
		return nil, malformed(0, "embedded EMF checksum")
	}
	return result[:len(result):len(result)], nil
}
