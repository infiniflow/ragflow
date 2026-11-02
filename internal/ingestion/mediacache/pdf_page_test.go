package mediacache

import (
	"errors"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	doctype "ragflow/internal/deepdoc/parser/type"
)

func TestPageStoreRoundTripAndPurge(t *testing.T) {
	ctx := taskContext("pages")
	backend := newFakeStore()
	store := NewPageStore(backend, "pages")
	ref := doctype.PageRef{Source: strings.Repeat("a", 64), Number: 2, Width: 3, Height: 2}
	img := image.NewRGBA(image.Rect(4, 5, 7, 7))
	img.SetRGBA(5, 6, color.RGBA{R: 123, A: 255})
	if err := store.Put(ctx, ref, img); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds() != image.Rect(0, 0, 3, 2) || got.At(1, 1) != img.At(5, 6) {
		t.Fatalf("bitmap differs: %v", got.Bounds())
	}
	if err := PurgeTask(ctx, backend, "pages"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, ref); !errors.Is(err, deepdocpdf.ErrPageCache) {
		t.Fatalf("missing page error: %v", err)
	}
}

func TestPageStoreRetryRenewsTTL(t *testing.T) {
	ctx := taskContext("pages")
	backend := newFakeStore()
	store := NewPageStore(backend, "pages")
	ref := doctype.PageRef{Source: strings.Repeat("f", 64), Width: 2, Height: 2}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if err := store.Put(ctx, ref, img); err != nil {
		t.Fatal(err)
	}
	digests, state := pageQuotaKeys("pages")
	keys := []string{store.key(ref), digests, state, manifestKey("pages")}
	for _, key := range keys {
		backend.ttl[key] = time.Hour
	}
	if err := store.Put(ctx, ref, img); err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if backend.ttl[key] != TTL {
			t.Fatalf("retry did not renew %s", key)
		}
	}
}

func TestPageStoreRejectsCorruptPageAndWriteFailure(t *testing.T) {
	ctx := taskContext("pages")
	backend := newFakeStore()
	store := NewPageStore(backend, "pages")
	ref := doctype.PageRef{Source: strings.Repeat("b", 64), Width: 2, Height: 2}
	backend.failSet = true
	if err := store.Put(ctx, ref, image.NewRGBA(image.Rect(0, 0, 2, 2))); !errors.Is(err, deepdocpdf.ErrPageCache) {
		t.Fatalf("write error: %v", err)
	}
	backend.failSet = false
	if err := store.Put(ctx, ref, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	backend.kv[store.key(ref)] = "RFP1\xff\xff\xff\xff"
	if _, err := store.Get(ctx, ref); !errors.Is(err, deepdocpdf.ErrPageCache) {
		t.Fatalf("corrupt page error: %v", err)
	}
}

func TestPageStoreRejectsChangedPixels(t *testing.T) {
	ctx := taskContext("pages")
	backend := newFakeStore()
	store := NewPageStore(backend, "pages")
	ref := doctype.PageRef{Source: strings.Repeat("c", 64), Width: 2, Height: 2}
	if err := store.Put(ctx, ref, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	data := []byte(backend.kv[store.key(ref)])
	data[len(data)-1] ^= 1
	backend.kv[store.key(ref)] = string(data)
	if _, err := store.Get(ctx, ref); !errors.Is(err, deepdocpdf.ErrPageCache) {
		t.Fatalf("changed pixels accepted: %v", err)
	}
	if err := store.Put(ctx, ref, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, ref); err != nil {
		t.Fatalf("parser retry did not repair page: %v", err)
	}
}

func TestPageStoreQuotaScopeAndRetry(t *testing.T) {
	ctx := taskContext("pages")
	backend := newFakeStore()
	store := NewPageStore(backend, "pages")
	store.maxBytes = 60
	ref := doctype.PageRef{Source: strings.Repeat("d", 64), Width: 2, Height: 2}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if err := store.Put(ctx, ref, img); err != nil {
		t.Fatal(err)
	}
	if backend.ttl[store.key(ref)] != TTL {
		t.Fatal("page TTL differs from media TTL")
	}
	delete(backend.kv, store.key(ref))
	if err := store.Put(ctx, ref, img); err != nil {
		t.Fatalf("retry did not restore page: %v", err)
	}
	if _, err := store.Get(ctx, ref); err != nil {
		t.Fatal(err)
	}
	ref.Number = 1
	if err := store.Put(ctx, ref, img); !errors.Is(err, deepdocpdf.ErrPageCache) {
		t.Fatalf("quota not enforced: %v", err)
	}
	if _, err := store.Get(taskContext("other"), ref); !errors.Is(err, deepdocpdf.ErrPageCache) {
		t.Fatalf("scope not enforced: %v", err)
	}
}
