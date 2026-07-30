import { IconFontFill } from '@/components/icon-font';
import SvgIcon from '@/components/svg-icon';
import queritLogo from '@/assets/querit.png';
import { cn } from '@/lib/utils';
import {
  Columns3Cog,
  FileCode,
  FileText,
  Globe,
  HousePlus,
  Infinity as InfinityIcon,
  LogOut,
  LucideBlocks,
  LucideFile,
  LucideFilePlay,
  LucideFileStack,
  LucideHeading,
  LucideListPlus,
} from 'lucide-react';
import { Component } from 'react';
import { Operator } from '../constants/agent';

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
  [Operator.BGPT]: 'bgpt',
  [Operator.SearXNG]: 'searxng',
  [Operator.KeenableSearch]: 'keenable',
  [Operator.TavilyExtract]: 'tavily',
  [Operator.TavilySearch]: 'tavily',
  [Operator.QueritSearch]: 'querit',
  [Operator.Wikipedia]: 'wikipedia',
  [Operator.YahooFinance]: 'yahoo-finance',
  [Operator.WenCai]: 'wencai',
  [Operator.Crawler]: 'crawler',
};
export const LucideIconMap = {
  [Operator.DataOperations]: FileCode,
  [Operator.Loop]: InfinityIcon,
  [Operator.ExitLoop]: LogOut,
  [Operator.DocGenerator]: FileText,
  [Operator.Browser]: Globe,
  [Operator.Compiler]: Columns3Cog,
  [Operator.File]: LucideFile,
  [Operator.Parser]: LucideFilePlay,
  [Operator.Tokenizer]: LucideListPlus,
  [Operator.TokenChunker]: LucideBlocks,
  [Operator.TitleChunker]: LucideHeading,
  [Operator.Extractor]: LucideFileStack,
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

  if (name === Operator.QueritSearch) {
    return (
      <img
        src={queritLogo}
        alt=""
        className={cn('size-5 object-contain', className)}
      />
    );
  }

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
