import { IconFont } from '@/components/icon-font';
import { cn } from '@/lib/utils';
import { CirclePlay } from 'lucide-react';
import { Operator } from './constant';

interface IProps {
  name: Operator;
  className?: string;
}

export const OperatorIconMap = {
  [Operator.Retrieval]: 'KR',
  // [Operator.Generate]: MergeCellsOutlined,
  // [Operator.Answer]: SendOutlined,
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
  // [Operator.Relevant]: BranchesOutlined,
  // [Operator.RewriteQuestion]: FormOutlined,
  // [Operator.KeywordExtract]: KeywordIcon,
  // [Operator.DuckDuckGo]: DuckIcon,
  // [Operator.Baidu]: BaiduIcon,
  // [Operator.Wikipedia]: WikipediaIcon,
  // [Operator.PubMed]: PubMedIcon,
  // [Operator.ArXiv]: ArXivIcon,
  // [Operator.Google]: GoogleIcon,
  // [Operator.Bing]: BingIcon,
  // [Operator.GoogleScholar]: GoogleScholarIcon,
  // [Operator.DeepL]: DeepLIcon,
  // [Operator.GitHub]: GitHubIcon,
  // [Operator.BaiduFanyi]: baiduFanyiIcon,
  // [Operator.QWeather]: QWeatherIcon,
  // [Operator.ExeSQL]: ExeSqlIcon,
  // [Operator.WenCai]: WenCaiIcon,
  // [Operator.AkShare]: AkShareIcon,
  // [Operator.YahooFinance]: YahooFinanceIcon,
  // [Operator.Jin10]: Jin10Icon,
  // [Operator.Concentrator]: ConcentratorIcon,
  // [Operator.TuShare]: TuShareIcon,
  // [Operator.Note]: NoteIcon,
  // [Operator.Crawler]: CrawlerIcon,
  // [Operator.Invoke]: InvokeIcon,
  // [Operator.Template]: TemplateIcon,
  // [Operator.Email]: EmailIcon,
  // [Operator.IterationStart]: CirclePower,
  // [Operator.WaitingDialogue]: MessageSquareMore,
};

const Empty = () => {
  return <div className="hidden"></div>;
};

const OperatorIcon = ({ name, className }: IProps) => {
  const Icon = OperatorIconMap[name as keyof typeof OperatorIconMap] || Empty;

  return typeof Icon === 'string' ? (
    <IconFont name={Icon} className={cn('size-5', className)}></IconFont>
  ) : (
    <Icon className={cn('size-5', className)}> </Icon>
  );
};

export default OperatorIcon;
