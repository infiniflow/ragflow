//go:build !ee

package parser

import "context"

func checkWPSLicense(context.Context, string) error { return nil }
