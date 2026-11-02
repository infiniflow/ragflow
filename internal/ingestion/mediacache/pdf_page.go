package mediacache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"os"
	doctype "ragflow/internal/deepdoc/parser/type"
	"strconv"
	"strings"

	"github.com/cespare/xxhash/v2"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	"ragflow/internal/ingestion/component/globals"
)

// PageStore stores PDF page bitmaps in the run media manifest with separate quotas.
type PageStore struct {
	backend  Backend
	scope    string
	maxBytes int64
}

func NewPageStore(backend Backend, scope string) *PageStore {
	limit := int64(64 << 30)
	if value, err := strconv.ParseInt(os.Getenv("RAGFLOW_PDF_PAGE_CACHE_MAX_BYTES"), 10, 64); err == nil && value > 0 {
		limit = value
	}
	return &PageStore{backend: backend, scope: strings.TrimSpace(scope), maxBytes: limit}
}

func (s *PageStore) key(ref doctype.PageRef) string {
	return fmt.Sprintf("mc:page:v1:216:%x:%s:%d", xxhash.Sum64String(s.scope), ref.Source, ref.Number)
}

func (s *PageStore) validate(ctx context.Context, ref doctype.PageRef) error {
	if s == nil || s.backend == nil || s.scope == "" {
		return fmt.Errorf("%w: store or run scope is absent", deepdocpdf.ErrPageCache)
	}
	if task := globals.TaskID(ctx); task != "" && task != s.scope {
		return fmt.Errorf("%w: task scope mismatch", deepdocpdf.ErrPageCache)
	}
	if err := ref.Validate(); err != nil {
		return fmt.Errorf("%w: %w", deepdocpdf.ErrPageCache, err)
	}
	return ctx.Err()
}

func (s *PageStore) Put(ctx context.Context, ref doctype.PageRef, img image.Image) error {
	if err := s.validate(ctx, ref); err != nil {
		return err
	}
	if img == nil || img.Bounds().Dx() != ref.Width || img.Bounds().Dy() != ref.Height {
		return fmt.Errorf("%w: bitmap dimensions differ", deepdocpdf.ErrPageCache)
	}
	data := make([]byte, 44+ref.Width*ref.Height*4)
	copy(data, "RFP1")
	binary.LittleEndian.PutUint32(data[4:8], uint32(ref.Width))
	binary.LittleEndian.PutUint32(data[8:12], uint32(ref.Height))
	raster := &image.RGBA{Pix: data[44:], Stride: ref.Width * 4, Rect: image.Rect(0, 0, ref.Width, ref.Height)}
	draw.Draw(raster, raster.Bounds(), img, img.Bounds().Min, draw.Src)
	digest := sha256.Sum256(data[44:])
	copy(data[12:44], digest[:])
	digests, state := pageQuotaKeys(s.scope)
	key := s.key(ref)
	value := string(data)
	write := func() (bool, bool, error) {
		return s.backend.MediaPut(ctx, key, digests, state, key, value, manifestKey(s.scope), key, 0, s.maxBytes, TTL)
	}
	accepted, inserted, err := write()
	if err == nil && accepted && !inserted {
		existing, readErr := s.backend.Get(ctx, key)
		if readErr != nil {
			return fmt.Errorf("%w: verify existing page: %w", deepdocpdf.ErrPageCache, readErr)
		}
		if existing != value {
			if !s.backend.Delete(ctx, key) {
				return fmt.Errorf("%w: remove stale page", deepdocpdf.ErrPageCache)
			}
			accepted, _, err = write()
		}
	}
	if err != nil {
		return fmt.Errorf("%w: write: %w", deepdocpdf.ErrPageCache, err)
	}
	if !accepted {
		return fmt.Errorf("%w: page cache exceeds %d bytes", deepdocpdf.ErrPageCache, s.maxBytes)
	}
	if !inserted {
		for _, key := range []string{key, digests, state, manifestKey(s.scope)} {
			if !s.backend.Expire(ctx, key, TTL) {
				return fmt.Errorf("%w: renew page lifecycle", deepdocpdf.ErrPageCache)
			}
		}
	}
	return nil
}

func (s *PageStore) Get(ctx context.Context, ref doctype.PageRef) (image.Image, error) {
	if err := s.validate(ctx, ref); err != nil {
		return nil, err
	}
	data, err := s.backend.Get(ctx, s.key(ref))
	if err != nil {
		return nil, fmt.Errorf("%w: read: %w", deepdocpdf.ErrPageCache, err)
	}
	if len(data) != 44+ref.Width*ref.Height*4 || data[:4] != "RFP1" {
		return nil, fmt.Errorf("%w: missing or corrupt bitmap", deepdocpdf.ErrPageCache)
	}
	digest := sha256.Sum256([]byte(data[44:]))
	if !bytes.Equal(digest[:], []byte(data[12:44])) {
		return nil, fmt.Errorf("%w: bitmap checksum differs", deepdocpdf.ErrPageCache)
	}
	var header [12]byte
	copy(header[:], data[:12])
	if int(binary.LittleEndian.Uint32(header[4:8])) != ref.Width || int(binary.LittleEndian.Uint32(header[8:12])) != ref.Height {
		return nil, fmt.Errorf("%w: bitmap dimensions differ", deepdocpdf.ErrPageCache)
	}
	raster := image.NewRGBA(image.Rect(0, 0, ref.Width, ref.Height))
	copy(raster.Pix, data[44:])
	return raster, nil
}

func pageQuotaKeys(scope string) (string, string) {
	prefix := fmt.Sprintf("mc:pagequota:%x", xxhash.Sum64String(scope))
	return prefix + ":digests", prefix + ":state"
}
