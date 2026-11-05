package common

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
)

const (
	DefaultPage     = 1
	DefaultPageSize = 30
	MaxPageSize     = 100
)

// ParsePagination parses REST list parameters. Only omitted parameters use defaults.
func ParsePagination(query url.Values) (int, int, error) {
	page, err := parsePaginationInt(query, "page", DefaultPage)
	if err != nil {
		return 0, 0, err
	}
	size, err := parsePaginationInt(query, "page_size", DefaultPageSize)
	if err != nil {
		return 0, 0, err
	}
	if err := ValidatePagination(&page, &size, nil); err != nil {
		return 0, 0, err
	}
	return page, size, nil
}

func parsePaginationInt(query url.Values, name string, fallback int) (int, error) {
	values, ok := query[name]
	if !ok {
		return fallback, nil
	}
	if len(values) != 1 {
		return 0, fmt.Errorf("%s must be provided once", name)
	}
	value, err := strconv.Atoi(strings.TrimSpace(values[0]))
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return value, nil
}

func validatePageSize(size int, name string) error {
	if size < 1 || size > MaxPageSize {
		return fmt.Errorf("%s must be between 1 and %d", name, MaxPageSize)
	}
	return nil
}

// ValidatePagination validates JSON pagination, including both size aliases when supplied.
func ValidatePagination(page, pageSize, size *int) error {
	p, s := DefaultPage, DefaultPageSize
	if page != nil {
		p = *page
	}
	if p < 1 {
		return fmt.Errorf("page must be a positive integer")
	}
	if size != nil {
		if err := validatePageSize(*size, "size"); err != nil {
			return err
		}
		s = *size
	}
	if pageSize != nil {
		if err := validatePageSize(*pageSize, "page_size"); err != nil {
			return err
		}
		s = *pageSize
	}
	// Bound the end of the page too, since some stores compute page * size.
	if p > math.MaxInt/s {
		return fmt.Errorf("pagination offset is too large")
	}
	return nil
}
