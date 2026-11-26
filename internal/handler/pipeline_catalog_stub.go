// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// Licensed under the Apache License, Version 2.0.

//go:build !ee

package handler

import pipelinepkg "ragflow/internal/ingestion/pipeline"

func filterLicensedPipelines(catalog *pipelinepkg.BuiltinPipelineListResponse) *pipelinepkg.BuiltinPipelineListResponse {
	return catalog
}
