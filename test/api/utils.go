// tests/api/utils.go
////go:build integration

package api

import "errors"

func ParsePriorityString(priority string) (int, error) {
	switch priority {
	case "p0":
		return 0, nil
	case "p1":
		return 1, nil
	case "p2":
		return 2, nil
	case "p3":
		return 3, nil
	default:
		return -1, errors.New("invalid priority")
	}
}
