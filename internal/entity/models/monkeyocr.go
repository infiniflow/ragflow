// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
package models

// MonkeyOCRModel reuses MinerU's local /file_parse ZIP client while
// identifying the provider as MonkeyOCR. Dispatch and UUID classification
// both key off Name(); NewInstance must preserve that identity.
type MonkeyOCRModel struct {
	*MinerULocalModel
}

func NewMonkeyOCRModel(baseURL map[string]string, urlSuffix URLSuffix) *MonkeyOCRModel {
	return &MonkeyOCRModel{MinerULocalModel: NewMinerLocalUModel(baseURL, urlSuffix)}
}

func (m *MonkeyOCRModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewMonkeyOCRModel(baseURL, m.baseModel.URLSuffix)
}

func (m *MonkeyOCRModel) Name() string {
	return "monkeyocr"
}
