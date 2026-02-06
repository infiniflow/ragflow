import { IconFontFill } from '@/components/icon-font';
import SvgIcon from '@/components/svg-icon';
import { cn } from '@/lib/utils';
import {
  FileCode,
  FileText,
  HousePlus,
  Infinity as InfinityIcon,
  LogOut,
} from 'lucide-react';
import { Component } from 'react';
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
  [Operator.ListOperations]: 'a-listoperations',
  [Operator.VariableAssigner]: 'a-ariableassigner',
  [Operator.VariableAggregator]: 'aggregator',
};

export const SVGIconMap = {
  [Operator.ArXiv]: 'arxiv',
  [Operator.GitHub]: 'github',
  [Operator.Bing]: 'bing',
  [Operator.DuckDuckGo]: 'duck',
  [Operator.Google]: 'google',
  [Operator.GoogleScholar]: 'google-scholar',
  [Operator.PubMed]: 'pubmed',
  [Operator.SearXNG]: 'searxng',
  [Operator.TavilyExtract]: 'tavily',
  [Operator.TavilySearch]: 'tavily',
  [Operator.Wikipedia]: 'wikipedia',
  [Operator.YahooFinance]: 'yahoo-finance',
  [Operator.WenCai]: 'wencai',
  [Operator.Crawler]: 'crawler',
};
export const LucideIconMap = {
  [Operator.DataOperations]: FileCode,
  [Operator.Loop]: InfinityIcon,
  [Operator.ExitLoop]: LogOut,
  [Operator.PDFGenerator]: FileText,
};

const Empty = () => {
  return <div className="hidden"></div>;
};

class SvgErrorBoundary extends Component<{
  children: React.ReactNode;
  fallback?: React.ReactNode;
}> {
  state = { hasError: false };

  static getDerivedStateFromError() {
    return { hasError: true };
  }

  render() {
    if (this.state.hasError) {
      return this.props.fallback || <Empty />;
    }

    return this.props.children;
  }
}

const OperatorIcon = ({ name, className }: IProps) => {
  const Icon = OperatorIconMap[name as keyof typeof OperatorIconMap];
  const svgIcon = SVGIconMap[name as keyof typeof SVGIconMap];
  const LucideIcon = LucideIconMap[name as keyof typeof LucideIconMap];

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

  if (Icon) {
    return (
      <SvgErrorBoundary fallback={<Empty />}>
        <IconFontFill
          name={Icon}
          className={cn('size-5 ', className)}
        ></IconFontFill>
      </SvgErrorBoundary>
    );
  }

  if (LucideIcon) {
    return (
      <SvgErrorBoundary fallback={<Empty />}>
        <LucideIcon className={cn('size-5', className)} />
      </SvgErrorBoundary>
    );
  }

  if (svgIcon) {
    return (
      <SvgErrorBoundary fallback={<Empty />}>
        <SvgIcon
          name={svgIcon}
          width={'100%'}
          className={cn('size-5 fill-current', className)}
        ></SvgIcon>
      </SvgErrorBoundary>
    );
  }

  return <Empty></Empty>;
};

export default OperatorIcon;
