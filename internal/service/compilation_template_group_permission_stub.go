//go:build !ee

package service

import "ragflow/internal/entity"

func authorizeCompilationTemplateGroupOwner(string, *entity.CompilationTemplateGroup) error {
	return nil
}
