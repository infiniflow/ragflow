//go:build cgo

package post

import (
	"fmt"

	"ragflow/internal/entity"
	"ragflow/internal/service"
)

func init() {
	resolveImageDescriber = func(tenantID, llmID string) (ImageDescriber, error) {
		svc := service.NewModelProviderService()
		driver, modelName, apiCfg, maxTokens, err := svc.GetModelConfigFromProviderInstance(
			tenantID, entity.ModelTypeImage2Text, llmID,
		)
		if err != nil {
			return nil, fmt.Errorf("resolve VLM model %q: %w", llmID, err)
		}
		return newModelImageDescriberFromDriver(driver, modelName, apiCfg, maxTokens), nil
	}
}
