package common

import (
	"fmt"
	"math"
	"net/url"
	"strings"
	"testing"
)

func TestParsePagination(t *testing.T) {
	for _, tt := range []struct {
		query      string
		page, size int
		wantErr    string
	}{
		{"", 1, 30, ""},
		{"page=2&page_size=100", 2, 100, ""},
		{"page_size=1", 1, 1, ""},
		{"page=3", 3, 30, ""},
		{"page_size=101", 0, 0, "page_size"},
		{"page_size=0", 0, 0, "page_size"},
		{"page_size=-1", 0, 0, "page_size"},
		{"page_size=abc", 0, 0, "page_size"},
		{"page_size=", 0, 0, "page_size"},
		{"page_size=1.5", 0, 0, "page_size"},
		{"page=0", 0, 0, "page"},
		{"page=-1", 0, 0, "page"},
		{"page=abc", 0, 0, "page"},
		{"page=", 0, 0, "page"},
		{"page=1&page=2", 0, 0, "provided once"},
		{"page_size=1&page_size=101", 0, 0, "provided once"},
		{fmt.Sprintf("page=%d&page_size=100", math.MaxInt), 0, 0, "offset"},
		{"page_size=999999999999999999999999999", 0, 0, "page_size"},
	} {
		t.Run(tt.query, func(t *testing.T) {
			query, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatal(err)
			}
			page, size, err := ParsePagination(query)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || page != tt.page || size != tt.size {
				t.Fatalf("page/size/error = %d/%d/%v, want %d/%d/nil", page, size, err, tt.page, tt.size)
			}
		})
	}
}

func TestValidatePaginationAliases(t *testing.T) {
	for _, tt := range []struct {
		name                 string
		page, pageSize, size *int
		valid                bool
	}{
		{"defaults", nil, nil, nil, true},
		{"max page_size", new(1), new(100), nil, true},
		{"max size", new(1), nil, new(100), true},
		{"oversized page_size", nil, new(101), nil, false},
		{"oversized size", nil, nil, new(101), false},
		{"ignored alias still validated", nil, new(10), new(101), false},
		{"zero size", nil, nil, new(0), false},
		{"negative page_size", nil, new(-1), nil, false},
		{"zero page", new(0), nil, nil, false},
		{"overflow with defaults", new(math.MaxInt), nil, nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePagination(tt.page, tt.pageSize, tt.size)
			if (err == nil) != tt.valid {
				t.Fatalf("error = %v, want valid %v", err, tt.valid)
			}
		})
	}
}
