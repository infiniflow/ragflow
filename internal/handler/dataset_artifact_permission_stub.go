//go:build !ee

package handler

import "github.com/gin-gonic/gin"

func readableArtifactSources(_ *gin.Context, _ string, groups [][]string) ([]bool, error) {
	visible := make([]bool, len(groups))
	for i := range visible {
		visible[i] = true
	}
	return visible, nil
}

func (h *DatasetArtifactHandler) artifactWriteAllowed(*gin.Context, string, []string) bool {
	return true
}
func (h *DatasetArtifactHandler) checkArtifactWrite(*gin.Context, string, []string) bool { return true }
func checkDatasetArtifactWrite(*gin.Context, string) bool                                { return true }
func checkDocumentGraphWrite(*gin.Context, string) error                                 { return nil }
func wikiPageSourceReadable(*gin.Context, string, string, string) bool                   { return true }
