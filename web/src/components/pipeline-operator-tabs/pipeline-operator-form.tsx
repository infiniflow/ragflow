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
import { RAGFlowNodeType } from '@/interfaces/database/agent';
import CompilationForm from '@/pages/agent/form/compilation-form';
import ExtractorForm from '@/pages/agent/form/extractor-form';
import ParserForm from '@/pages/agent/form/parser-form';
import TitleChunkerForm from '@/pages/agent/form/title-chunker-form';
import TokenChunkerForm from '@/pages/agent/form/token-chunker-form';
import TokenizerForm from '@/pages/agent/form/tokenizer-form';
import { getOperatorType } from '@/utils/pipeline-operator';
import { memo, useCallback } from 'react';
import { FieldErrors } from 'react-hook-form';

type PipelineOperatorFormProps = {
  node: RAGFlowNodeType;
  onValuesChange?: (values: any) => void;
  externalErrors?: FieldErrors;
};

const PipelineOperatorForm = ({
  node,
  onValuesChange,
  externalErrors,
}: PipelineOperatorFormProps) => {
  const operatorType = getOperatorType(
    (node.data as Record<string, any>)?.operatorId || node.data?.label || '',
  );

  const handleValuesChange = useCallback(
    (values: any) => {
      onValuesChange?.(values);
    },
    [onValuesChange],
  );

  switch (operatorType) {
    case Operator.Parser:
      return (
        <ParserForm
          node={node}
          onValuesChange={handleValuesChange}
          hideOutputs
          externalErrors={externalErrors}
        />
      );
    case Operator.TokenChunker:
      return (
        <TokenChunkerForm
          node={node}
          onValuesChange={handleValuesChange}
          hideOutputs
          externalErrors={externalErrors}
        />
      );
    case Operator.TitleChunker:
      return (
        <TitleChunkerForm
          node={node}
          onValuesChange={handleValuesChange}
          hideOutputs
          externalErrors={externalErrors}
        />
      );
    case Operator.Extractor:
      return (
        <ExtractorForm
          node={node}
          onValuesChange={handleValuesChange}
          hideOutputs
          externalErrors={externalErrors}
        />
      );
    case Operator.Compiler:
      return (
        <CompilationForm
          node={node}
          onValuesChange={handleValuesChange}
          hideOutputs
          externalErrors={externalErrors}
        />
      );
    case Operator.Tokenizer:
      return (
        <TokenizerForm
          node={node}
          onValuesChange={handleValuesChange}
          hideOutputs
          externalErrors={externalErrors}
        />
      );
    default:
      return null;
  }
};

export default memo(PipelineOperatorForm);
