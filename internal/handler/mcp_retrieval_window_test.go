package handler

import (
	"math"
	"strings"
	"testing"
)

func TestValidateRetrievalWindow(t *testing.T) {
	tests := []struct {
		name     string
		page     int
		pageSize int
		wantErr  string
	}{
		{name: "defaults fit the window", page: 0, pageSize: 0}, // treated as 1 x 30
		{name: "first page at schema max", page: 1, pageSize: 100},
		{name: "paging stays inside the window", page: 5, pageSize: 100},
		{name: "exact boundary is allowed", page: 8, pageSize: 64}, // 8 * 64 == 512
		{
			name:     "one past the boundary is rejected",
			page:     9,
			pageSize: 64,
			wantErr:  "exceeds the fixed rerank candidate window",
		},
		{
			name:     "window past the fixed candidates is rejected",
			page:     mcpRerankCandidatesCount/100 + 1,
			pageSize: 100,
			wantErr:  "exceeds the fixed rerank candidate window",
		},
		{
			name:     "max int page cannot overflow the check",
			page:     math.MaxInt,
			pageSize: math.MaxInt,
			wantErr:  "exceeds the fixed rerank candidate window",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRetrievalWindow(tt.page, tt.pageSize)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateRetrievalWindow(%d, %d) = %v, want nil", tt.page, tt.pageSize, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateRetrievalWindow(%d, %d) = %v, want error containing %q", tt.page, tt.pageSize, err, tt.wantErr)
			}
		})
	}
}
