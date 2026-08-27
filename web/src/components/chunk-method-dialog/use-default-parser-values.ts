/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { IParserConfig } from '@/interfaces/database/document';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { ParseDocumentType } from '../layout-recognize-form-field';

export function useDefaultParserValues() {
  const { t } = useTranslation();

  const defaultParserValues = useMemo(() => {
    const defaultParserValues = {
      task_page_size: 12,
      layout_recognize: ParseDocumentType.DeepDOC,
      chunk_token_num: 512,
      delimiter: '\n',
      enable_children: false,
      children_delimiter: '\n',
      auto_keywords: 0,
      auto_questions: 0,
      html4excel: false,
      image_table_context_window: 0,
      mineru_parse_method: 'auto',
      mineru_formula_enable: true,
      mineru_table_enable: true,
      mineru_lang: 'English',
      raptor: {
        use_raptor: false,
        prompt: t('knowledgeConfiguration.promptText'),
        max_token: 256,
        threshold: 0.1,
        max_cluster: 64,
        random_seed: 0,
        scope: 'file',
        clustering_method: 'gmm',
        tree_builder: 'raptor',
      },
      // graphrag: {
      //   use_graphrag: false,
      // },
      entity_types: [],
      pages: [],
      metadata: [],
      built_in_metadata: [],
      enable_metadata: false,
    };

    return defaultParserValues as IParserConfig;
  }, [t]);

  return defaultParserValues;
}

export function useFillDefaultValueOnMount() {
  const defaultParserValues = useDefaultParserValues();

  const fillDefaultValue = useCallback(
    (parserConfig: IParserConfig) => {
      return Object.entries(defaultParserValues).reduce<Record<string, any>>(
        (pre, [key, value]) => {
          if (key in parserConfig) {
            pre[key] = parserConfig[key as keyof IParserConfig];
          } else {
            pre[key] = value;
          }
          return pre;
        },
        {},
      );
    },
    [defaultParserValues],
  );

  return fillDefaultValue;
}
