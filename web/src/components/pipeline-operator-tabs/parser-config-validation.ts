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

import { Operator } from '@/constants/agent';
import {
  initialCompilationValues,
  initialGoExtractorValues,
  initialParserValues,
  initialTitleChunkerValues,
  initialTokenChunkerValues,
  initialTokenizerValues,
} from '@/pages/agent/constant/pipeline';
import { buildCompilationFormSchema } from '@/pages/agent/form/compilation-form';
import { FormSchema as ExtractorFormSchema } from '@/pages/agent/form/extractor-form';
import { FormSchema as ParserFormSchema } from '@/pages/agent/form/parser-form';
import { FormSchema as TitleChunkerFormSchema } from '@/pages/agent/form/title-chunker-form';
import { FormSchema as TokenChunkerFormSchema } from '@/pages/agent/form/token-chunker-form';
import { FormSchema as TokenizerFormSchema } from '@/pages/agent/form/tokenizer-form';
import { getOperatorType } from '@/utils/pipeline-operator';
import type { TFunction } from 'i18next';
import { z } from 'zod';

// The pipeline operator tabs (and therefore this validation) only render for
// the Go backend — the Python backend edits the parser config through the
// legacy chunk-method dialog — so only the Go form schemas are mapped here.
function getOperatorFormSchema(operatorType: Operator, t: TFunction) {
  switch (operatorType) {
    case Operator.Parser:
      return ParserFormSchema;
    case Operator.TokenChunker:
      return TokenChunkerFormSchema;
    case Operator.TitleChunker:
      return TitleChunkerFormSchema;
    case Operator.Extractor:
      return ExtractorFormSchema;
    case Operator.Compiler:
      return buildCompilationFormSchema(t);
    case Operator.Tokenizer:
      return TokenizerFormSchema;
    default:
      return undefined;
  }
}

function getOperatorInitialValues(operatorType: Operator) {
  switch (operatorType) {
    case Operator.Parser:
      return initialParserValues;
    case Operator.TokenChunker:
      return initialTokenChunkerValues;
    case Operator.TitleChunker:
      return initialTitleChunkerValues;
    case Operator.Extractor:
      return initialGoExtractorValues;
    case Operator.Compiler:
      return initialCompilationValues;
    case Operator.Tokenizer:
      return initialTokenizerValues;
    default:
      return {};
  }
}

/**
 * Validates every operator config in parser_config against the owning
 * operator form's schema, so the required rules defined by the operator
 * forms also hold when the forms are submitted from an outer form (document
 * pipeline dialog, dataset setting page) whose resolver is the only one that
 * runs on submit.
 *
 * Entries may come from a saved config (never rendered in a tab) or straight
 * from an operator form, so the operator defaults are merged in first:
 * untouched tabs validate against their defaults instead of false-positive
 * missing fields, while edited tabs carry complete form values that override
 * the defaults and get genuinely validated.
 */
export function addParserConfigIssues(
  parserConfig: Record<string, any> | undefined,
  ctx: z.RefinementCtx,
  t: TFunction,
) {
  if (!parserConfig) {
    return;
  }

  for (const [operatorId, config] of Object.entries(parserConfig)) {
    const operatorType = getOperatorType(operatorId);
    const schema = getOperatorFormSchema(operatorType, t);
    if (!schema) {
      continue;
    }

    const result = schema.safeParse({
      ...getOperatorInitialValues(operatorType),
      ...(config ?? {}),
    });
    if (result.success) {
      continue;
    }

    for (const issue of result.error.issues) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['parser_config', operatorId, ...issue.path],
        message: issue.message,
      });
    }
  }
}
