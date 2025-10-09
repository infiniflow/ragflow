import { IconFont } from '@/components/icon-font';
import { cn } from '@/lib/utils';
import {
  Blocks,
  File,
  FileChartColumnIncreasing,
  FileStack,
  Heading,
  ListMinus,
} from 'lucide-react';
import { Operator } from './constant';

interface IProps {
  name: Operator;
  className?: string;
}

export const OperatorIconMap = {
  [Operator.Note]: 'notebook-pen',
};

export const SVGIconMap = {
  [Operator.Begin]: File,
  [Operator.Parser]: FileChartColumnIncreasing,
  [Operator.Tokenizer]: ListMinus,
  [Operator.Splitter]: Blocks,
  [Operator.HierarchicalMerger]: Heading,
  [Operator.Extractor]: FileStack,
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
        <File className="rounded size-3" />
      </div>
    );
  }

  return typeof Icon === 'string' ? (
    <IconFont name={Icon} className={cn('size-5 ', className)}></IconFont>
  ) : (
    <SvgIcon className={cn('size-5', className)}></SvgIcon>
  );
};

export default OperatorIcon;
