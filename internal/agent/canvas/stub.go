//go:build !ee

package canvas

import "context"

func pushFilteredEvent(context.Context, chan<- RunEvent, RunEvent) bool { return false }
