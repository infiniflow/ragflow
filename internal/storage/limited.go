package storage

import (
	"context"
	"errors"
	"io"
)

// ErrObjectTooLarge reports that a bounded storage read would exceed its limit.
var ErrObjectTooLarge = errors.New("storage object exceeds read limit")

// LimitedGetter retrieves an object without buffering more than maxBytes plus
// one byte, which is enough to distinguish an exact fit from an oversized object.
type LimitedGetter interface {
	GetLimited(ctx context.Context, bucket, fnm string, maxBytes int64, tenantID ...string) ([]byte, error)
}

func readLimitedObject(reader io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes < 0 {
		return nil, errors.New("storage read limit must not be negative")
	}
	if maxBytes == int64(^uint64(0)>>1) {
		return io.ReadAll(reader)
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrObjectTooLarge
	}
	return data, nil
}
