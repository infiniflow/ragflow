package infinity

import (
	"context"
	"errors"
	"testing"
	"time"

	infinity "github.com/infiniflow/infinity-go-sdk"
)

// wrapTestErr simulates an error that kept its chain across a wrapping layer.
type wrapTestErr struct{ inner error }

func (w wrapTestErr) Error() string { return "open chunk table: " + w.inner.Error() }
func (w wrapTestErr) Unwrap() error { return w.inner }

func TestIsTableMissingError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"typed table-not-exist", infinity.NewInfinityException(int(infinity.ErrorCodeTableNotExist), "Table t1 doesn't exist."), true},
		{"typed other code", infinity.NewInfinityException(7018, "Failed to connect to server"), false},
		{"wrapped typed table-not-exist", wrapTestErr{infinity.NewInfinityException(int(infinity.ErrorCodeTableNotExist), "Table t1 doesn't exist.")}, true},
		{"string doesn't exist", errors.New("Table ragflow_x_y doesn't exist."), true},
		{"string not found", errors.New("DB::Table not found: ragflow_x_y"), true},
		{"unrelated", errors.New("connection refused"), false},
	}
	for _, tc := range cases {
		if got := isTableMissingError(tc.err); got != tc.want {
			t.Errorf("%s: isTableMissingError = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestRetryOnTableMissingRecoversAfterBlips(t *testing.T) {
	for _, blips := range []int{0, 1, 2} {
		calls := 0
		table, err := retryOnTableMissing(context.Background(), func() (*infinity.Table, error) {
			calls++
			if calls <= blips {
				return nil, infinity.NewInfinityException(int(infinity.ErrorCodeTableNotExist), "Table t1 doesn't exist.")
			}
			return nil, nil // a nil table stands in for a successful open
		})
		if err != nil {
			t.Fatalf("blips=%d: unexpected error: %v", blips, err)
		}
		if table != nil {
			t.Fatalf("blips=%d: expected nil table stand-in", blips)
		}
		if calls != blips+1 {
			t.Fatalf("blips=%d: calls = %d, want %d", blips, calls, blips+1)
		}
	}
}

func TestRetryOnTableMissingReturnsOtherErrorsImmediately(t *testing.T) {
	calls := 0
	_, err := retryOnTableMissing(context.Background(), func() (*infinity.Table, error) {
		calls++
		return nil, errors.New("connection refused")
	})
	if err == nil || err.Error() != "connection refused" {
		t.Fatalf("err = %v, want connection refused", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (non-missing errors must not retry)", calls)
	}
}

func TestRetryOnTableMissingSurfacesGenuinelyMissingTable(t *testing.T) {
	calls := 0
	start := time.Now()
	_, err := retryOnTableMissing(context.Background(), func() (*infinity.Table, error) {
		calls++
		return nil, infinity.NewInfinityException(int(infinity.ErrorCodeTableNotExist), "Table t1 doesn't exist.")
	})
	if err == nil {
		t.Fatal("expected the missing-table error to surface after the retries")
	}
	if calls != len(tableMissingBackoffs)+1 {
		t.Fatalf("calls = %d, want %d", calls, len(tableMissingBackoffs)+1)
	}
	if elapsed := time.Since(start); elapsed < tableMissingBackoffs[0]+tableMissingBackoffs[1] {
		t.Fatalf("elapsed = %v, want at least the full backoff sequence %v", elapsed, tableMissingBackoffs[0]+tableMissingBackoffs[1])
	}
}

func TestRetryOnTableMissingHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	_, err := retryOnTableMissing(ctx, func() (*infinity.Table, error) {
		calls++
		cancel() // cancel while the first backoff is pending
		return nil, infinity.NewInfinityException(int(infinity.ErrorCodeTableNotExist), "Table t1 doesn't exist.")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (no further open after cancellation)", calls)
	}
}
