import { IconFont } from '@/components/icon-font';
import { cn } from '@/lib/utils';
import {
  FileChartColumnIncreasing,
  Grid3x3,
  HousePlus,
  ListMinus,
} from 'lucide-react';
import { Operator } from './constant';

interface IProps {
  name: Operator;
  className?: string;
}

export const OperatorIconMap = {
  [Operator.Retrieval]: 'KR',
  [Operator.Begin]: 'house-plus',
  [Operator.Categorize]: 'a-QuestionClassification',
  [Operator.Message]: 'reply',
  [Operator.Iteration]: 'loop',
  [Operator.Switch]: 'condition',
  [Operator.Code]: 'code-set',
  [Operator.Agent]: 'agent-ai',
  [Operator.UserFillUp]: 'await',
  [Operator.StringTransform]: 'a-textprocessing',
  [Operator.Note]: 'notebook-pen',
  [Operator.ExeSQL]: 'executesql-0',
  [Operator.Invoke]: 'httprequest-0',
  [Operator.Email]: 'sendemail-0',
};

export const SVGIconMap = {
  [Operator.Parser]: FileChartColumnIncreasing,
  [Operator.Chunker]: Grid3x3,
  [Operator.Tokenizer]: ListMinus,
};

const Empty = () => {
  return <div className="hidden"></div>;
};

const OperatorIcon = ({ name, className }: IProps) => {
  const Icon = OperatorIconMap[name as keyof typeof OperatorIconMap] || Empty;
  const SvgIcon = SVGIconMap[name as keyof typeof SVGIconMap] || Empty;

  if (name === Operator.Begin) {
    return (
      <div
        className={cn(
          'inline-block p-1 bg-accent-primary rounded-sm',
          className,
        )}
      >
        <HousePlus className="rounded size-3" />
      </div>
    );
  }

  return typeof Icon === 'string' ? (
    <IconFont name={Icon} className={cn('size-5 ', className)}></IconFont>
  ) : (
    <SvgIcon className="size-5"></SvgIcon>
  );
};

export default OperatorIcon;
