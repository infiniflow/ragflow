//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package infinity

import (
	"errors"
	"testing"

	infinitysdk "github.com/infiniflow/infinity-go-sdk"
)

// wrappedError stands in for the layers that wrap an engine error with %w; the
// message deliberately omits the Infinity startup wording so the code path is
// exercised on its own.
type wrappedError struct{ err error }

func (w wrappedError) Error() string { return "execute pipeline: " + w.err.Error() }
func (w wrappedError) Unwrap() error { return w.err }

func TestIsTransientUnavailable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{
			name: "starting code",
			err:  infinitysdk.NewInfinityException(int(infinitysdk.ErrorCodeInfinityIsStarting), "engine not ready"),
			want: true,
		},
		{
			name: "initing code",
			err:  infinitysdk.NewInfinityException(int(infinitysdk.ErrorCodeInfinityIsIniting), "engine not ready"),
			want: true,
		},
		{
			name: "wrapped code",
			err:  wrappedError{infinitysdk.NewInfinityException(int(infinitysdk.ErrorCodeInfinityIsIniting), "engine not ready")},
			want: true,
		},
		{
			name: "other code",
			err:  infinitysdk.NewInfinityException(int(infinitysdk.ErrorCodeTableNotExist), "Table not exist"),
			want: false,
		},
		{name: "rendered initing", err: errors.New("rpc error: InfinityException(2008, Infinity is initing)"), want: true},
		{name: "rendered starting", err: errors.New("execute: InfinityException(2007, Infinity is starting)"), want: true},
		{name: "quoted text", err: errors.New("document body says Infinity is initing"), want: false},
		{name: "unrelated", err: errors.New("boom"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsTransientUnavailable(tc.err); got != tc.want {
				t.Fatalf("IsTransientUnavailable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
