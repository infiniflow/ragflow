//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package dao

import (
	"ragflow/internal/model"
)

// LLMDAO LLM data access object
type LLMDAO struct{}

// NewLLMDAO create LLM DAO
func NewLLMDAO() *LLMDAO {
	return &LLMDAO{}
}

// GetAll gets all LLMs
func (dao *LLMDAO) GetAll() ([]*model.LLM, error) {
	var llms []*model.LLM
	err := DB.Find(&llms).Error
	if err != nil {
		return nil, err
	}
	return llms, nil
}

// GetAllValid gets all valid LLMs
func (dao *LLMDAO) GetAllValid() ([]*model.LLM, error) {
	var llms []*model.LLM
	err := DB.Where("status = ?", "1").Find(&llms).Error
	if err != nil {
		return nil, err
	}
	return llms, nil
}

// GetByFactory gets LLMs by factory
func (dao *LLMDAO) GetByFactory(factory string) ([]*model.LLM, error) {
	var llms []*model.LLM
	err := DB.Where("fid = ?", factory).Find(&llms).Error
	if err != nil {
		return nil, err
	}
	return llms, nil
}

// GetByFactoryAndName gets LLM by factory and name
func (dao *LLMDAO) GetByFactoryAndName(factory, name string) (*model.LLM, error) {
	var llm model.LLM
	err := DB.Where("fid = ? AND llm_name = ?", factory, name).First(&llm).Error
	if err != nil {
		return nil, err
	}
	return &llm, nil
}

// LLMFactoryDAO LLM factory data access object
type LLMFactoryDAO struct{}

// NewLLMFactoryDAO create LLM factory DAO
func NewLLMFactoryDAO() *LLMFactoryDAO {
	return &LLMFactoryDAO{}
}

// GetAllValid gets all valid LLM factories
func (dao *LLMFactoryDAO) GetAllValid() ([]*model.LLMFactories, error) {
	var factories []*model.LLMFactories
	err := DB.Where("status = ?", "1").Find(&factories).Error
	if err != nil {
		return nil, err
	}
	return factories, nil
}

// GetByName gets LLM factory by name
func (dao *LLMFactoryDAO) GetByName(name string) (*model.LLMFactories, error) {
	var factory model.LLMFactories
	err := DB.Where("name = ?", name).First(&factory).Error
	if err != nil {
		return nil, err
	}
	return &factory, nil
}
