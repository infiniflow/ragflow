package pdf

import (
	"context"
	"errors"
	"fmt"
	"image"
	"sync"

	doctype "ragflow/internal/deepdoc/parser/type"
)

var ErrPageCache = errors.New("pdf page cache failure")

// PageCache hands lossless page bitmaps from the parser to its consumers.
type PageCache interface {
	Put(context.Context, doctype.PageRef, image.Image) error
	Get(context.Context, doctype.PageRef) (image.Image, error)
}

type pageCacheKey struct{}
type pageDocumentKey struct{}
type pageDocumentState struct {
	mu       sync.Mutex
	document doctype.PageDocument
}

func WithPageCache(ctx context.Context, cache PageCache) context.Context {
	return context.WithValue(ctx, pageCacheKey{}, cache)
}

func PageCacheFromContext(ctx context.Context) PageCache {
	cache, _ := ctx.Value(pageCacheKey{}).(PageCache)
	return cache
}

func WithPageDocument(ctx context.Context, document doctype.PageDocument) context.Context {
	pages := make(map[int]doctype.PageSize, len(document.Pages))
	for number, ref := range document.Pages {
		pages[number] = ref
	}
	document.Pages = pages
	return context.WithValue(ctx, pageDocumentKey{}, &pageDocumentState{document: document})
}

func HasPageDocument(ctx context.Context) bool {
	return ctx.Value(pageDocumentKey{}) != nil
}

func PageDocumentFromContext(ctx context.Context) *doctype.PageDocument {
	state, _ := ctx.Value(pageDocumentKey{}).(*pageDocumentState)
	if state == nil {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	document := state.document
	document.Pages = make(map[int]doctype.PageSize, len(state.document.Pages))
	for number, ref := range state.document.Pages {
		document.Pages[number] = ref
	}
	return &document
}

func cachePage(ctx context.Context, number int, img image.Image) error {
	state, _ := ctx.Value(pageDocumentKey{}).(*pageDocumentState)
	if state == nil {
		return nil
	}
	cache := PageCacheFromContext(ctx)
	if cache == nil {
		return fmt.Errorf("%w: not configured", ErrPageCache)
	}
	bounds := img.Bounds()
	ref := doctype.PageRef{Source: state.document.Source, Number: number, Width: bounds.Dx(), Height: bounds.Dy()}
	if err := cache.Put(ctx, ref, img); err != nil {
		return fmt.Errorf("%w: publish page %d: %w", ErrPageCache, number, err)
	}
	state.mu.Lock()
	state.document.Pages[number] = doctype.PageSize{Width: ref.Width, Height: ref.Height}
	state.mu.Unlock()
	return nil
}

func LoadCachedPage(ctx context.Context, number int) (image.Image, error) {
	state, _ := ctx.Value(pageDocumentKey{}).(*pageDocumentState)
	if state == nil {
		return nil, fmt.Errorf("%w: document is absent", ErrPageCache)
	}
	state.mu.Lock()
	ref, available := state.document.Page(number)
	state.mu.Unlock()
	if !available {
		return nil, nil
	}
	cache := PageCacheFromContext(ctx)
	if cache == nil {
		return nil, fmt.Errorf("%w: not configured", ErrPageCache)
	}
	img, err := cache.Get(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("%w: load page %d: %w", ErrPageCache, number, err)
	}
	return img, nil
}
