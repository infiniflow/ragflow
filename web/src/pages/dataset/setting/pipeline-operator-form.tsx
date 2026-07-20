import { Operator } from '@/constants/agent';
import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { memo, useCallback } from 'react';
import ExtractorForm from '../../agent/form/extractor-form';
import ParserForm from '../../agent/form/parser-form';
import TitleChunkerForm from '../../agent/form/title-chunker-form';
import TokenChunkerForm from '../../agent/form/token-chunker-form';
import TokenizerForm from '../../agent/form/tokenizer-form';
import { getOperatorType } from './utils';

type PipelineOperatorFormProps = {
  node: RAGFlowNodeType;
  onValuesChange?: (values: any) => void;
};

const PipelineOperatorForm = ({
  node,
  onValuesChange,
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
        />
      );
    case Operator.TokenChunker:
      return (
        <TokenChunkerForm
          node={node}
          onValuesChange={handleValuesChange}
          hideOutputs
        />
      );
    case Operator.TitleChunker:
      return (
        <TitleChunkerForm
          node={node}
          onValuesChange={handleValuesChange}
          hideOutputs
        />
      );
    case Operator.Extractor:
      return (
        <ExtractorForm
          node={node}
          onValuesChange={handleValuesChange}
          hideOutputs
        />
      );
    case Operator.Tokenizer:
      return (
        <TokenizerForm
          node={node}
          onValuesChange={handleValuesChange}
          hideOutputs
        />
      );
    default:
      return null;
  }
};

export default memo(PipelineOperatorForm);
