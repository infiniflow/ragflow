//go:build !ee

package service

func ingestionErrorCode(error) string { return "" }
