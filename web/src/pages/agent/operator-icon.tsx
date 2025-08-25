import { ReactComponent as ArxivIcon } from '@/assets/svg/arxiv.svg';
import { ReactComponent as BingIcon } from '@/assets/svg/bing.svg';
import { ReactComponent as CrawlerIcon } from '@/assets/svg/crawler.svg';
import { ReactComponent as DuckIcon } from '@/assets/svg/duck.svg';
import { ReactComponent as GithubIcon } from '@/assets/svg/github.svg';
import { ReactComponent as GoogleScholarIcon } from '@/assets/svg/google-scholar.svg';
import { ReactComponent as GoogleIcon } from '@/assets/svg/google.svg';
import { ReactComponent as PubMedIcon } from '@/assets/svg/pubmed.svg';
import { ReactComponent as TavilyIcon } from '@/assets/svg/tavily.svg';
import { ReactComponent as WenCaiIcon } from '@/assets/svg/wencai.svg';
import { ReactComponent as WikipediaIcon } from '@/assets/svg/wikipedia.svg';
import { ReactComponent as YahooFinanceIcon } from '@/assets/svg/yahoo-finance.svg';

import { IconFont } from '@/components/icon-font';
import { cn } from '@/lib/utils';
import { HousePlus } from 'lucide-react';
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
  [Operator.ArXiv]: ArxivIcon,
  [Operator.GitHub]: GithubIcon,
  [Operator.Bing]: BingIcon,
  [Operator.DuckDuckGo]: DuckIcon,
  [Operator.Google]: GoogleIcon,
  [Operator.GoogleScholar]: GoogleScholarIcon,
  [Operator.PubMed]: PubMedIcon,
  [Operator.TavilyExtract]: TavilyIcon,
  [Operator.TavilySearch]: TavilyIcon,
  [Operator.Wikipedia]: WikipediaIcon,
  [Operator.YahooFinance]: YahooFinanceIcon,
  [Operator.WenCai]: WenCaiIcon,
  [Operator.Crawler]: CrawlerIcon,
};

const Empty = () => {
  return <div className="hidden"></div>;
};

const OperatorIcon = ({ name, className }: IProps) => {
  const Icon = OperatorIconMap[name as keyof typeof OperatorIconMap] || Empty;
  const SvgIcon = SVGIconMap[name as keyof typeof SVGIconMap] || Empty;

  if (name === Operator.Begin) {
    return (
      <div className="inline-block p-1 bg-accent-primary rounded-sm">
        <HousePlus className="rounded size-3" />
      </div>
    );
  }

  return typeof Icon === 'string' ? (
    <IconFont name={Icon} className={cn('size-5 ', className)}></IconFont>
  ) : (
    <SvgIcon className={cn('size-5 fill-current', className)}></SvgIcon>
  );
};

export default OperatorIcon;
