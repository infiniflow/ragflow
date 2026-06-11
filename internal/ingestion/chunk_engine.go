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

package ingestion

import "ragflow/internal/ingestion/chunk"

/*
{
  "version": "1.0",
  "name": "media_aware_chunking",
  "description": "遇到图片/视频 URL 时禁用 overlap",

  "pipeline": [
    {
      "stage": "preprocess",
      "normalize_newlines": true,
      "strip_whitespace": true,
      "remove_empty_lines": true
    },

    {
      "stage": "split",
      "strategy": "sentence",
      "params": {
        "boundaries": ["。", "！", "？", "\n"],
        "keep_separators": true
      }
    },

    {
      "stage": "postprocess",
      "merge": {
        "target_size": 500,
        "strategy": "greedy"
      },
      "overlap": {
        "unit": "char",
        "mode": "if_only",
        "conditions": [
          {
            "name": "包含媒体URL",
            "if": "has_media_url = true",
            "then": {"size": 0}
          },
          {
            "name": "包含图片URL",
            "if": "has_image_url = true",
            "then": {"size": 0}
          },
          {
            "name": "包含视频URL",
            "if": "has_video_url = true",
            "then": {"size": 0}
          },
          {
            "name": "普通中文长句子",
            "if": "language = 'zh' AND length > 50 AND has_media_url = false",
            "then": {"size": 1, "unit": "sentence"}
          },
          {
            "name": "普通中文短句子",
            "if": "language = 'zh' AND length <= 50 AND has_media_url = false",
            "then": {"size": 30}
          }
        ],
        "default": {"size": 50}
      },
      "filter": {
        "min_length": 10,
        "max_length": 1200
      },
      "add_metadata": {
        "include_index": true,
        "custom_fields": {
          "has_media_url": "auto_detect"
        }
      }
    }
  ]
}
*/

type ChunkPlan struct {
	Operators []chunk.Operator
}

type ChunkEngine struct {
}

func NewChunkEngine() *ChunkEngine {
	return &ChunkEngine{}
}

func (e *ChunkEngine) Plan(dsl *string) (*ChunkPlan, error) {
	return nil, nil
}

func (e *ChunkEngine) Execute(chunk *ChunkPlan) error {
	return nil
}

func (e *ChunkEngine) Explain(chunk *ChunkPlan) error {
	return nil
}
