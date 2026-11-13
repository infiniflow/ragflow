package util

import (
	"encoding/base64"
	"image"
	"testing"
)

func TestEncodePNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	data, err := EncodePNG(img)
	if err != nil {
		t.Fatalf("EncodePNG: %v", err)
	}
	if len(data) == 0 {
		t.Error("encoded PNG should not be empty")
	}
}

func TestDecodeBase64PNG(t *testing.T) {
	// Encode → base64 → decode roundtrip
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	data, _ := EncodePNG(img)
	b64 := base64.StdEncoding.EncodeToString(data)
	decoded, err := DecodeBase64PNG(b64)
	if err != nil {
		t.Fatalf("DecodeBase64PNG: %v", err)
	}
	if decoded.Bounds() != img.Bounds() {
		t.Errorf("bounds mismatch: got %v, want %v", decoded.Bounds(), img.Bounds())
	}

	// Invalid base64
	_, err = DecodeBase64PNG("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64")
	}

	// Valid base64 but not PNG
	b64NotPNG := base64.StdEncoding.EncodeToString([]byte("not a png"))
	_, err = DecodeBase64PNG(b64NotPNG)
	if err == nil {
		t.Error("expected error for non-PNG data")
	}
}
