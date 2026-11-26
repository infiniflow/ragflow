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
