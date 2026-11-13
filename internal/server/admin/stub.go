//go:build !ee

package admin

import "context"

func startMetrics(context.Context) func() { return func() {} }
