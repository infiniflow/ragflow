import { ReactComponent as ArxivIcon } from '@/assets/svg/arxiv.svg';
import { ReactComponent as BingIcon } from '@/assets/svg/bing.svg';
import { ReactComponent as CrawlerIcon } from '@/assets/svg/crawler.svg';
import { ReactComponent as DuckIcon } from '@/assets/svg/duck.svg';
import { ReactComponent as GithubIcon } from '@/assets/svg/github.svg';
import { ReactComponent as GoogleScholarIcon } from '@/assets/svg/google-scholar.svg';
import { ReactComponent as GoogleIcon } from '@/assets/svg/google.svg';
import { ReactComponent as PubMedIcon } from '@/assets/svg/pubmed.svg';
import { ReactComponent as SearXNGIcon } from '@/assets/svg/searxng.svg';
import { ReactComponent as TavilyIcon } from '@/assets/svg/tavily.svg';
import { ReactComponent as WenCaiIcon } from '@/assets/svg/wencai.svg';
import { ReactComponent as WikipediaIcon } from '@/assets/svg/wikipedia.svg';
import { ReactComponent as YahooFinanceIcon } from '@/assets/svg/yahoo-finance.svg';

import { IconFontFill } from '@/components/icon-font';
import { cn } from '@/lib/utils';
import { FileCode, HousePlus } from 'lucide-react';
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
  [Operator.ArXiv]: ArxivIcon,
  [Operator.GitHub]: GithubIcon,
  [Operator.Bing]: BingIcon,
  [Operator.DuckDuckGo]: DuckIcon,
  [Operator.Google]: GoogleIcon,
  [Operator.GoogleScholar]: GoogleScholarIcon,
  [Operator.PubMed]: PubMedIcon,
  [Operator.SearXNG]: SearXNGIcon,
  [Operator.TavilyExtract]: TavilyIcon,
  [Operator.TavilySearch]: TavilyIcon,
  [Operator.Wikipedia]: WikipediaIcon,
  [Operator.YahooFinance]: YahooFinanceIcon,
  [Operator.WenCai]: WenCaiIcon,
  [Operator.Crawler]: CrawlerIcon,
};
export const LucideIconMap = {
  [Operator.DataOperations]: FileCode,
};

const Empty = () => {
  return <div className="hidden"></div>;
};

const OperatorIcon = ({ name, className }: IProps) => {
  const Icon = OperatorIconMap[name as keyof typeof OperatorIconMap];
  const SvgIcon = SVGIconMap[name as keyof typeof SVGIconMap];
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
      <IconFontFill
        name={Icon}
        className={cn('size-5 ', className)}
      ></IconFontFill>
    );
  }

  if (LucideIcon) {
    return <LucideIcon className={cn('size-5', className)} />;
  }

  if (SvgIcon) {
    return <SvgIcon className={cn('size-5 fill-current', className)}></SvgIcon>;
  }

  return <Empty></Empty>;
};

export default OperatorIcon;
