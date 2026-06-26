//go:build !cgo

package post

import "errors"

func init() {
	resolveImageDescriber = func(tenantID, llmID string) (ImageDescriber, error) {
		return nil, errors.New("VLM requires CGO (image2text model)")
	}
}
