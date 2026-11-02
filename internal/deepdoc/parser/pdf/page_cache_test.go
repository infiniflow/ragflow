package pdf

import (
	"context"
	"errors"
	"image"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	doctype "ragflow/internal/deepdoc/parser/type"
)

type testPageCache struct {
	mu     sync.Mutex
	putErr error
	puts   int
	image  image.Image
}

func (c *testPageCache) Put(_ context.Context, _ doctype.PageRef, img image.Image) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.puts++
	c.image = img
	return c.putErr
}
func (c *testPageCache) Get(context.Context, doctype.PageRef) (image.Image, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.image, nil
}

func TestParserPublishesDefaultPageAndPropagatesCacheError(t *testing.T) {
	cache := &testPageCache{}
	ctx := WithPageCache(t.Context(), cache)
	ctx = WithPageDocument(ctx, doctype.PageDocument{Source: strings.Repeat("a", 64)})
	engine := &MockEngine{RenderPageImageFunc: func(int, float64) (image.Image, error) { return image.NewRGBA(image.Rect(0, 0, 3, 2)), nil }}
	parser := &Parser{}
	if _, err := parser.renderPageToImage(ctx, engine, 0); err != nil {
		t.Fatal(err)
	}
	if cache.puts != 1 || len(PageDocumentFromContext(ctx).Pages) != 1 {
		t.Fatalf("page not published: %d", cache.puts)
	}
	cache.putErr = errors.New("connection lost")
	if _, err := parser.renderPageToImage(ctx, engine, 1); !errors.Is(err, ErrPageCache) {
		t.Fatalf("error swallowed: %v", err)
	}
	if len(PageDocumentFromContext(ctx).Pages) != 1 {
		t.Fatal("failed page was published")
	}
}

func TestParserPageCacheFailureIsFatalAndRetryDPIIsSeparate(t *testing.T) {
	cache := &testPageCache{putErr: errors.New("write failed")}
	ctx := WithPageDocument(WithPageCache(t.Context(), cache), doctype.PageDocument{Source: strings.Repeat("b", 64)})
	parser := &Parser{}
	if _, err := parser.ParseRaw(ctx, &MockEngine{}, nil); !errors.Is(err, ErrPageCache) {
		t.Fatalf("ParseRaw swallowed cache failure: %v", err)
	}
	cache.putErr = nil
	if _, err := parser.renderAtDPI(ctx, &MockEngine{}, 0, 432); err != nil {
		t.Fatal(err)
	}
	if cache.puts != 1 || len(PageDocumentFromContext(ctx).Pages) != 0 {
		t.Fatal("retry render changed default-DPI page cache")
	}
}

func TestParserStopsAfterPageCacheFailure(t *testing.T) {
	cache := &testPageCache{putErr: errors.New("quota exceeded")}
	ctx := WithPageDocument(WithPageCache(t.Context(), cache), doctype.PageDocument{Source: strings.Repeat("e", 64)})
	var renders atomic.Int64
	engine := &MockEngine{NumPages: 100, RenderPageImageFunc: func(int, float64) (image.Image, error) {
		renders.Add(1)
		return image.NewRGBA(image.Rect(0, 0, 3, 2)), nil
	}}
	if _, err := (&Parser{}).ParseRaw(ctx, engine, nil); !errors.Is(err, ErrPageCache) {
		t.Fatalf("fatal error lost: %v", err)
	}
	if renders.Load() >= 100 {
		t.Fatal("parser rendered every remaining page after cache failure")
	}
}
