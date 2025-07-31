import { IconFont } from '@/components/icon-font';
import SvgIcon from '@/components/svg-icon';
import { cn } from '@/lib/utils';
import { CirclePlay } from 'lucide-react';
import { Operator } from './constant';

interface IProps {
  name: Operator;
  className?: string;
}

export const OperatorIconMap = {
  [Operator.Retrieval]: 'KR',
  [Operator.Begin]: CirclePlay,
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

const SVGIconMap = {
  [Operator.ArXiv]: 'arxiv',
  [Operator.GitHub]: 'github',
  [Operator.Bing]: 'bing',
  [Operator.DuckDuckGo]: 'duck',
  [Operator.Google]: 'google',
  [Operator.GoogleScholar]: 'google-scholar',
  [Operator.PubMed]: 'pubmed',
  [Operator.TavilyExtract]: 'tavily',
  [Operator.TavilySearch]: 'tavily',
  [Operator.Wikipedia]: 'wikipedia',
  [Operator.YahooFinance]: 'yahoo-finance',
  [Operator.WenCai]: 'wencai',
  [Operator.Crawler]: 'crawler',
};

const Empty = () => {
  return <div className="hidden"></div>;
};

const OperatorIcon = ({ name, className }: IProps) => {
  const Icon = OperatorIconMap[name as keyof typeof OperatorIconMap] || Empty;

  if (name in SVGIconMap) {
    return (
      <SvgIcon
        name={SVGIconMap[name as keyof typeof SVGIconMap]}
        width={20}
        className={className}
      ></SvgIcon>
    );
  }

  return typeof Icon === 'string' ? (
    <IconFont name={Icon} className={cn('size-5', className)}></IconFont>
  ) : (
    <Icon width={20} className={cn('size-5 fill-current', className)}></Icon>
  );
};

export default OperatorIcon;
