//go:build !ee

package component

func publishPreviewFile(map[string]any, fileInputs)       {}
func previewFileInputs(map[string]any) (fileInputs, bool) { return fileInputs{}, false }

func embeddingEnabledForRun(kbID string) bool { return true }
