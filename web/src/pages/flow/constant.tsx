import {
  GitHubIcon,
  KeywordIcon,
  QWeatherIcon,
  WikipediaIcon,
} from '@/assets/icon/next-icon';
import { ReactComponent as AkShareIcon } from '@/assets/svg/akshare.svg';
import { ReactComponent as ArXivIcon } from '@/assets/svg/arxiv.svg';
import { ReactComponent as baiduFanyiIcon } from '@/assets/svg/baidu-fanyi.svg';
import { ReactComponent as BaiduIcon } from '@/assets/svg/baidu.svg';
import { ReactComponent as BeginIcon } from '@/assets/svg/begin.svg';
import { ReactComponent as BingIcon } from '@/assets/svg/bing.svg';
import { ReactComponent as ConcentratorIcon } from '@/assets/svg/concentrator.svg';
import { ReactComponent as CrawlerIcon } from '@/assets/svg/crawler.svg';
import { ReactComponent as DeepLIcon } from '@/assets/svg/deepl.svg';
import { ReactComponent as DuckIcon } from '@/assets/svg/duck.svg';
import { ReactComponent as EmailIcon } from '@/assets/svg/email.svg';
import { ReactComponent as ExeSqlIcon } from '@/assets/svg/exesql.svg';
import { ReactComponent as GoogleScholarIcon } from '@/assets/svg/google-scholar.svg';
import { ReactComponent as GoogleIcon } from '@/assets/svg/google.svg';
import { ReactComponent as InvokeIcon } from '@/assets/svg/invoke-ai.svg';
import { ReactComponent as Jin10Icon } from '@/assets/svg/jin10.svg';
import { ReactComponent as NoteIcon } from '@/assets/svg/note.svg';
import { ReactComponent as PubMedIcon } from '@/assets/svg/pubmed.svg';
import { ReactComponent as SwitchIcon } from '@/assets/svg/switch.svg';
import { ReactComponent as TemplateIcon } from '@/assets/svg/template.svg';
import { ReactComponent as TuShareIcon } from '@/assets/svg/tushare.svg';
import { ReactComponent as WenCaiIcon } from '@/assets/svg/wencai.svg';
import { ReactComponent as YahooFinanceIcon } from '@/assets/svg/yahoo-finance.svg';
import { CodeTemplateStrMap, ProgrammingLanguage } from '@/constants/agent';

// 邮件功能

import {
  ChatVariableEnabledField,
  variableEnabledFieldMap,
} from '@/constants/chat';
import i18n from '@/locales/config';
import { setInitialChatVariableEnabledFieldValue } from '@/utils/chat';

// DuckDuckGo's channel options
export enum Channel {
  Text = 'text',
  News = 'news',
}

import {
  BranchesOutlined,
  DatabaseOutlined,
  FormOutlined,
  MergeCellsOutlined,
  MessageOutlined,
  RocketOutlined,
  SendOutlined,
} from '@ant-design/icons';
import upperFirst from 'lodash/upperFirst';
import {
  CirclePower,
  CloudUpload,
  CodeXml,
  Database,
  IterationCcw,
  ListOrdered,
  OptionIcon,
  TextCursorInput,
  ToggleLeft,
  WrapText,
} from 'lucide-react';

export const BeginId = 'begin';

export enum Operator {
  Begin = 'Begin',
  Retrieval = 'Retrieval',
  Generate = 'Generate',
  Answer = 'Answer',
  Categorize = 'Categorize',
  Message = 'Message',
  Relevant = 'Relevant',
  RewriteQuestion = 'RewriteQuestion',
  KeywordExtract = 'KeywordExtract',
  Baidu = 'Baidu',
  DuckDuckGo = 'DuckDuckGo',
  Wikipedia = 'Wikipedia',
  PubMed = 'PubMed',
  ArXiv = 'ArXiv',
  Google = 'Google',
  Bing = 'Bing',
  GoogleScholar = 'GoogleScholar',
  DeepL = 'DeepL',
  GitHub = 'GitHub',
  BaiduFanyi = 'BaiduFanyi',
  QWeather = 'QWeather',
  ExeSQL = 'ExeSQL',
  Switch = 'Switch',
  WenCai = 'WenCai',
  AkShare = 'AkShare',
  YahooFinance = 'YahooFinance',
  Jin10 = 'Jin10',
  Concentrator = 'Concentrator',
  TuShare = 'TuShare',
  Note = 'Note',
  Crawler = 'Crawler',
  Invoke = 'Invoke',
  Template = 'Template',
  Email = 'Email',
  Iteration = 'Iteration',
  IterationStart = 'IterationItem',
  Code = 'Code',
}

export const CommonOperatorList = Object.values(Operator).filter(
  (x) => x !== Operator.Note,
);

export const operatorIconMap = {
  [Operator.Retrieval]: RocketOutlined,
  [Operator.Generate]: MergeCellsOutlined,
  [Operator.Answer]: SendOutlined,
  [Operator.Begin]: BeginIcon,
  [Operator.Categorize]: DatabaseOutlined,
  [Operator.Message]: MessageOutlined,
  [Operator.Relevant]: BranchesOutlined,
  [Operator.RewriteQuestion]: FormOutlined,
  [Operator.KeywordExtract]: KeywordIcon,
  [Operator.DuckDuckGo]: DuckIcon,
  [Operator.Baidu]: BaiduIcon,
  [Operator.Wikipedia]: WikipediaIcon,
  [Operator.PubMed]: PubMedIcon,
  [Operator.ArXiv]: ArXivIcon,
  [Operator.Google]: GoogleIcon,
  [Operator.Bing]: BingIcon,
  [Operator.GoogleScholar]: GoogleScholarIcon,
  [Operator.DeepL]: DeepLIcon,
  [Operator.GitHub]: GitHubIcon,
  [Operator.BaiduFanyi]: baiduFanyiIcon,
  [Operator.QWeather]: QWeatherIcon,
  [Operator.ExeSQL]: ExeSqlIcon,
  [Operator.Switch]: SwitchIcon,
  [Operator.WenCai]: WenCaiIcon,
  [Operator.AkShare]: AkShareIcon,
  [Operator.YahooFinance]: YahooFinanceIcon,
  [Operator.Jin10]: Jin10Icon,
  [Operator.Concentrator]: ConcentratorIcon,
  [Operator.TuShare]: TuShareIcon,
  [Operator.Note]: NoteIcon,
  [Operator.Crawler]: CrawlerIcon,
  [Operator.Invoke]: InvokeIcon,
  [Operator.Template]: TemplateIcon,
  [Operator.Email]: EmailIcon,
  [Operator.Iteration]: IterationCcw,
  [Operator.IterationStart]: CirclePower,
  [Operator.Code]: CodeXml,
};

export const operatorMap: Record<
  Operator,
  {
    backgroundColor?: string;
    color?: string;
    width?: number;
    height?: number;
    fontSize?: number;
    iconFontSize?: number;
    iconWidth?: number;
    moreIconColor?: string;
  }
> = {
  [Operator.Retrieval]: {
    backgroundColor: '#cad6e0',
    color: '#385974',
  },
  [Operator.Generate]: {
    backgroundColor: '#ebd6d6',
    width: 150,
    height: 150,
    fontSize: 20,
    iconFontSize: 30,
    color: '#996464',
  },
  [Operator.Answer]: {
    backgroundColor: '#f4816d',
    color: '#f4816d',
  },
  [Operator.Begin]: {
    backgroundColor: '#4f51d6',
  },
  [Operator.Categorize]: {
    backgroundColor: '#ffebcd',
    color: '#cc8a26',
  },
  [Operator.Message]: {
    backgroundColor: '#c5ddc7',
    color: 'green',
  },
  [Operator.Relevant]: {
    backgroundColor: '#9fd94d',
    color: '#8ef005',
    width: 70,
    height: 70,
    fontSize: 12,
    iconFontSize: 16,
  },
  [Operator.RewriteQuestion]: {
    backgroundColor: '#f8c7f8',
    color: '#f32bf3',
    width: 70,
    height: 70,
    fontSize: 12,
    iconFontSize: 16,
  },
  [Operator.KeywordExtract]: {
    width: 70,
    height: 70,
    backgroundColor: '#6E5494',
    color: '#6E5494',
    fontSize: 12,
    iconWidth: 16,
  },
  [Operator.DuckDuckGo]: {
    backgroundColor: '#e7e389',
    color: '#aea00c',
  },
  [Operator.Baidu]: {
    backgroundColor: '#d9e0f8',
  },
  [Operator.Wikipedia]: {
    backgroundColor: '#dee0e2',
  },
  [Operator.PubMed]: {
    backgroundColor: '#a2ccf0',
  },
  [Operator.ArXiv]: {
    width: 70,
    height: 70,
    fontSize: 12,
    iconWidth: 16,
    iconFontSize: 16,
    moreIconColor: 'white',
    backgroundColor: '#b31b1b',
    color: 'white',
  },
  [Operator.Google]: {
    backgroundColor: 'pink',
  },
  [Operator.Bing]: {
    backgroundColor: '#c0dcc4',
  },
  [Operator.GoogleScholar]: {
    backgroundColor: '#b4e4f6',
  },
  [Operator.DeepL]: {
    backgroundColor: '#f5e8e6',
  },
  [Operator.GitHub]: {
    backgroundColor: 'purple',
    color: 'purple',
  },
  [Operator.BaiduFanyi]: { backgroundColor: '#e5f2d3' },
  [Operator.QWeather]: {
    backgroundColor: '#a4bbf3',
    color: '#a4bbf3',
  },
  [Operator.ExeSQL]: { backgroundColor: '#b9efe8' },
  [Operator.Switch]: { backgroundColor: '#dbaff6', color: '#dbaff6' },
  [Operator.WenCai]: { backgroundColor: '#faac5b' },
  [Operator.AkShare]: { backgroundColor: '#8085f5' },
  [Operator.YahooFinance]: { backgroundColor: '#b474ff' },
  [Operator.Jin10]: { backgroundColor: '#a0b9f8' },
  [Operator.Concentrator]: {
    backgroundColor: '#32d2a3',
    color: '#32d2a3',
    width: 70,
    height: 70,
    fontSize: 10,
    iconFontSize: 16,
  },
  [Operator.TuShare]: { backgroundColor: '#f8cfa0' },
  [Operator.Note]: { backgroundColor: '#f8cfa0' },
  [Operator.Crawler]: {
    backgroundColor: '#dee0e2',
  },
  [Operator.Invoke]: {
    backgroundColor: '#dee0e2',
  },
  [Operator.Template]: {
    backgroundColor: '#dee0e2',
  },
  [Operator.Email]: { backgroundColor: '#e6f7ff' },
  [Operator.Iteration]: { backgroundColor: '#e6f7ff' },
  [Operator.IterationStart]: { backgroundColor: '#e6f7ff' },
  [Operator.Code]: { backgroundColor: '#4c5458' },
};

export const componentMenuList = [
  {
    name: Operator.Retrieval,
  },
  {
    name: Operator.Generate,
  },
  {
    name: Operator.Answer,
  },
  {
    name: Operator.Categorize,
  },
  {
    name: Operator.Message,
  },

  {
    name: Operator.RewriteQuestion,
  },
  {
    name: Operator.KeywordExtract,
  },
  {
    name: Operator.Switch,
  },
  {
    name: Operator.Concentrator,
  },
  {
    name: Operator.Template,
  },
  {
    name: Operator.Iteration,
  },
  {
    name: Operator.Code,
  },
  {
    name: Operator.Note,
  },
  {
    name: Operator.DuckDuckGo,
  },
  {
    name: Operator.Baidu,
  },
  {
    name: Operator.Wikipedia,
  },
  {
    name: Operator.PubMed,
  },
  {
    name: Operator.ArXiv,
  },
  {
    name: Operator.Google,
  },
  {
    name: Operator.Bing,
  },
  {
    name: Operator.GoogleScholar,
  },
  {
    name: Operator.DeepL,
  },
  {
    name: Operator.GitHub,
  },
  {
    name: Operator.BaiduFanyi,
  },
  {
    name: Operator.QWeather,
  },
  {
    name: Operator.ExeSQL,
  },
  {
    name: Operator.WenCai,
  },
  {
    name: Operator.AkShare,
  },
  {
    name: Operator.YahooFinance,
  },
  {
    name: Operator.Jin10,
  },
  {
    name: Operator.TuShare,
  },
  {
    name: Operator.Crawler,
  },
  {
    name: Operator.Invoke,
  },
  {
    name: Operator.Email,
  },
];

const initialQueryBaseValues = {
  query: [],
};

export const initialRetrievalValues = {
  similarity_threshold: 0.2,
  keywords_similarity_weight: 0.3,
  top_n: 8,
  use_kg: false,
  ...initialQueryBaseValues,
};

export const initialBeginValues = {
  prologue: `Hi! I'm your assistant. What can I do for you?`,
};

export const variableCheckBoxFieldMap = Object.keys(
  variableEnabledFieldMap,
).reduce<Record<string, boolean>>((pre, cur) => {
  pre[cur] = setInitialChatVariableEnabledFieldValue(
    cur as ChatVariableEnabledField,
  );
  return pre;
}, {});

const initialLlmBaseValues = {
  ...variableCheckBoxFieldMap,
  temperature: 0.1,
  top_p: 0.3,
  frequency_penalty: 0.7,
  presence_penalty: 0.4,
  max_tokens: 256,
};

export const initialGenerateValues = {
  ...initialLlmBaseValues,
  prompt: i18n.t('flow.promptText'),
  cite: true,
  message_history_window_size: 12,
  parameters: [],
};

export const initialRewriteQuestionValues = {
  ...initialLlmBaseValues,
  language: '',
  message_history_window_size: 6,
};

export const initialRelevantValues = {
  ...initialLlmBaseValues,
};

export const initialCategorizeValues = {
  ...initialLlmBaseValues,
  message_history_window_size: 1,
  category_description: {},
  ...initialQueryBaseValues,
};

export const initialMessageValues = {
  messages: [],
};

export const initialKeywordExtractValues = {
  ...initialLlmBaseValues,
  top_n: 3,
  ...initialQueryBaseValues,
};
export const initialDuckValues = {
  top_n: 10,
  channel: Channel.Text,
  ...initialQueryBaseValues,
};

export const initialBaiduValues = {
  top_n: 10,
  ...initialQueryBaseValues,
};

export const initialWikipediaValues = {
  top_n: 10,
  language: 'en',
  ...initialQueryBaseValues,
};

export const initialPubMedValues = {
  top_n: 10,
  email: '',
  ...initialQueryBaseValues,
};

export const initialArXivValues = {
  top_n: 10,
  sort_by: 'relevance',
  ...initialQueryBaseValues,
};

export const initialGoogleValues = {
  top_n: 10,
  api_key: 'YOUR_API_KEY (obtained from https://serpapi.com/manage-api-key)',
  country: 'cn',
  language: 'en',
  ...initialQueryBaseValues,
};

export const initialBingValues = {
  top_n: 10,
  channel: 'Webpages',
  api_key:
    'YOUR_API_KEY (obtained from https://www.microsoft.com/en-us/bing/apis/bing-web-search-api)',
  country: 'CH',
  language: 'en',
  ...initialQueryBaseValues,
};

export const initialGoogleScholarValues = {
  top_n: 5,
  sort_by: 'relevance',
  patents: true,
  ...initialQueryBaseValues,
};

export const initialDeepLValues = {
  top_n: 5,
  auth_key: 'relevance',
};

export const initialGithubValues = {
  top_n: 5,
  ...initialQueryBaseValues,
};

export const initialBaiduFanyiValues = {
  appid: 'xxx',
  secret_key: 'xxx',
  trans_type: 'translate',
  ...initialQueryBaseValues,
};

export const initialQWeatherValues = {
  web_apikey: 'xxx',
  type: 'weather',
  user_type: 'free',
  time_period: 'now',
  ...initialQueryBaseValues,
};

export const initialExeSqlValues = {
  ...initialLlmBaseValues,
  db_type: 'mysql',
  database: '',
  username: '',
  host: '',
  port: 3306,
  password: '',
  loop: 3,
  top_n: 30,
  ...initialQueryBaseValues,
};

export const initialSwitchValues = { conditions: [] };

export const initialWenCaiValues = {
  top_n: 20,
  query_type: 'stock',
  ...initialQueryBaseValues,
};

export const initialAkShareValues = { top_n: 10, ...initialQueryBaseValues };

export const initialYahooFinanceValues = {
  info: true,
  history: false,
  financials: false,
  balance_sheet: false,
  cash_flow_statement: false,
  news: true,
  ...initialQueryBaseValues,
};

export const initialJin10Values = {
  type: 'flash',
  secret_key: 'xxx',
  flash_type: '1',
  contain: '',
  filter: '',
  ...initialQueryBaseValues,
};

export const initialConcentratorValues = {};

export const initialTuShareValues = {
  token: 'xxx',
  src: 'eastmoney',
  start_date: '2024-01-01 09:00:00',
  ...initialQueryBaseValues,
};

export const initialNoteValues = {
  text: '',
};

export const initialCrawlerValues = {
  extract_type: 'markdown',
  ...initialQueryBaseValues,
};

export const initialInvokeValues = {
  url: 'http://',
  method: 'GET',
  timeout: 60,
  headers: `{
  "Accept": "*/*",
  "Cache-Control": "no-cache",
  "Connection": "keep-alive"
}`,
  proxy: 'http://',
  clean_html: false,
  datatype: 'json',
};

export const initialTemplateValues = {
  content: '',
  parameters: [],
};

export const initialEmailValues = {
  smtp_server: '',
  smtp_port: 587,
  email: '',
  password: '',
  sender_name: '',
  to_email: '',
  cc_email: '',
  subject: '',
  content: '',
};

export const initialIterationValues = {
  delimiter: ',',
};
export const initialIterationStartValues = {};

export const initialCodeValues = {
  lang: 'python',
  script: CodeTemplateStrMap[ProgrammingLanguage.Python],
  arguments: [
    {
      name: 'arg1',
    },
    {
      name: 'arg2',
    },
  ],
};

export const CategorizeAnchorPointPositions = [
  { top: 1, right: 34 },
  { top: 8, right: 18 },
  { top: 15, right: 10 },
  { top: 24, right: 4 },
  { top: 31, right: 1 },
  { top: 38, right: -2 },
  { top: 62, right: -2 }, //bottom
  { top: 71, right: 1 },
  { top: 79, right: 6 },
  { top: 86, right: 12 },
  { top: 91, right: 20 },
  { top: 98, right: 34 },
];

// key is the source of the edge, value is the target of the edge
// no connection lines are allowed between key and value
export const RestrictedUpstreamMap = {
  [Operator.Begin]: [Operator.Relevant],
  [Operator.Categorize]: [
    Operator.Begin,
    Operator.Categorize,
    Operator.Answer,
    Operator.Relevant,
  ],
  [Operator.Answer]: [
    Operator.Begin,
    Operator.Answer,
    Operator.Message,
    Operator.Relevant,
  ],
  [Operator.Retrieval]: [Operator.Begin, Operator.Retrieval],
  [Operator.Generate]: [Operator.Begin, Operator.Relevant],
  [Operator.Message]: [
    Operator.Begin,
    Operator.Message,
    Operator.Generate,
    Operator.Retrieval,
    Operator.RewriteQuestion,
    Operator.Categorize,
    Operator.Relevant,
  ],
  [Operator.Relevant]: [Operator.Begin, Operator.Answer, Operator.Relevant],
  [Operator.RewriteQuestion]: [
    Operator.Begin,
    Operator.Message,
    Operator.RewriteQuestion,
    Operator.Relevant,
  ],
  [Operator.KeywordExtract]: [
    Operator.Begin,
    Operator.Message,
    Operator.Relevant,
  ],
  [Operator.Baidu]: [Operator.Begin, Operator.Retrieval],
  [Operator.DuckDuckGo]: [Operator.Begin, Operator.Retrieval],
  [Operator.Wikipedia]: [Operator.Begin, Operator.Retrieval],
  [Operator.PubMed]: [Operator.Begin, Operator.Retrieval],
  [Operator.ArXiv]: [Operator.Begin, Operator.Retrieval],
  [Operator.Google]: [Operator.Begin, Operator.Retrieval],
  [Operator.Bing]: [Operator.Begin, Operator.Retrieval],
  [Operator.GoogleScholar]: [Operator.Begin, Operator.Retrieval],
  [Operator.DeepL]: [Operator.Begin, Operator.Retrieval],
  [Operator.GitHub]: [Operator.Begin, Operator.Retrieval],
  [Operator.BaiduFanyi]: [Operator.Begin, Operator.Retrieval],
  [Operator.QWeather]: [Operator.Begin, Operator.Retrieval],
  [Operator.ExeSQL]: [Operator.Begin],
  [Operator.Switch]: [Operator.Begin],
  [Operator.WenCai]: [Operator.Begin],
  [Operator.AkShare]: [Operator.Begin],
  [Operator.YahooFinance]: [Operator.Begin],
  [Operator.Jin10]: [Operator.Begin],
  [Operator.Concentrator]: [Operator.Begin],
  [Operator.TuShare]: [Operator.Begin],
  [Operator.Crawler]: [Operator.Begin],
  [Operator.Note]: [],
  [Operator.Invoke]: [Operator.Begin],
  [Operator.Template]: [Operator.Begin, Operator.Relevant],
  [Operator.Email]: [Operator.Begin],
  [Operator.Iteration]: [Operator.Begin],
  [Operator.IterationStart]: [Operator.Begin],
  [Operator.Code]: [Operator.Begin],
};

export const NodeMap = {
  [Operator.Begin]: 'beginNode',
  [Operator.Categorize]: 'categorizeNode',
  [Operator.Retrieval]: 'retrievalNode',
  [Operator.Generate]: 'generateNode',
  [Operator.Answer]: 'logicNode',
  [Operator.Message]: 'messageNode',
  [Operator.Relevant]: 'relevantNode',
  [Operator.RewriteQuestion]: 'rewriteNode',
  [Operator.KeywordExtract]: 'keywordNode',
  [Operator.DuckDuckGo]: 'ragNode',
  [Operator.Baidu]: 'ragNode',
  [Operator.Wikipedia]: 'ragNode',
  [Operator.PubMed]: 'ragNode',
  [Operator.ArXiv]: 'ragNode',
  [Operator.Google]: 'ragNode',
  [Operator.Bing]: 'ragNode',
  [Operator.GoogleScholar]: 'ragNode',
  [Operator.DeepL]: 'ragNode',
  [Operator.GitHub]: 'ragNode',
  [Operator.BaiduFanyi]: 'ragNode',
  [Operator.QWeather]: 'ragNode',
  [Operator.ExeSQL]: 'ragNode',
  [Operator.Switch]: 'switchNode',
  [Operator.Concentrator]: 'logicNode',
  [Operator.WenCai]: 'ragNode',
  [Operator.AkShare]: 'ragNode',
  [Operator.YahooFinance]: 'ragNode',
  [Operator.Jin10]: 'ragNode',
  [Operator.TuShare]: 'ragNode',
  [Operator.Note]: 'noteNode',
  [Operator.Crawler]: 'ragNode',
  [Operator.Invoke]: 'invokeNode',
  [Operator.Template]: 'templateNode',
  [Operator.Email]: 'emailNode',
  [Operator.Iteration]: 'group',
  [Operator.IterationStart]: 'iterationStartNode',
  [Operator.Code]: 'ragNode',
};

export const LanguageOptions = [
  {
    value: 'af',
    label: 'Afrikaans',
  },
  {
    value: 'pl',
    label: 'Polski',
  },
  {
    value: 'ar',
    label: 'العربية',
  },
  {
    value: 'ast',
    label: 'Asturianu',
  },
  {
    value: 'az',
    label: 'Azərbaycanca',
  },
  {
    value: 'bg',
    label: 'Български',
  },
  {
    value: 'nan',
    label: '閩南語 / Bân-lâm-gú',
  },
  {
    value: 'bn',
    label: 'বাংলা',
  },
  {
    value: 'be',
    label: 'Беларуская',
  },
  {
    value: 'ca',
    label: 'Català',
  },
  {
    value: 'cs',
    label: 'Čeština',
  },
  {
    value: 'cy',
    label: 'Cymraeg',
  },
  {
    value: 'da',
    label: 'Dansk',
  },
  {
    value: 'de',
    label: 'Deutsch',
  },
  {
    value: 'fr',
    label: 'Français',
  },
  {
    value: 'et',
    label: 'Eesti',
  },
  {
    value: 'el',
    label: 'Ελληνικά',
  },
  {
    value: 'en',
    label: 'English',
  },
  {
    value: 'es',
    label: 'Español',
  },
  {
    value: 'eo',
    label: 'Esperanto',
  },
  {
    value: 'eu',
    label: 'Euskara',
  },
  {
    value: 'fa',
    label: 'فارسی',
  },
  {
    value: 'fr',
    label: 'Français',
  },
  {
    value: 'gl',
    label: 'Galego',
  },
  {
    value: 'ko',
    label: '한국어',
  },
  {
    value: 'hy',
    label: 'Հայերեն',
  },
  {
    value: 'hi',
    label: 'हिन्दी',
  },
  {
    value: 'hr',
    label: 'Hrvatski',
  },
  {
    value: 'id',
    label: 'Bahasa Indonesia',
  },
  {
    value: 'it',
    label: 'Italiano',
  },
  {
    value: 'he',
    label: 'עברית',
  },
  {
    value: 'ka',
    label: 'ქართული',
  },
  {
    value: 'lld',
    label: 'Ladin',
  },
  {
    value: 'la',
    label: 'Latina',
  },
  {
    value: 'lv',
    label: 'Latviešu',
  },
  {
    value: 'lt',
    label: 'Lietuvių',
  },
  {
    value: 'hu',
    label: 'Magyar',
  },
  {
    value: 'mk',
    label: 'Македонски',
  },
  {
    value: 'arz',
    label: 'مصرى',
  },
  {
    value: 'ms',
    label: 'Bahasa Melayu',
  },
  {
    value: 'min',
    label: 'Bahaso Minangkabau',
  },
  {
    value: 'my',
    label: 'မြန်မာဘာသာ',
  },
  {
    value: 'nl',
    label: 'Nederlands',
  },
  {
    value: 'ja',
    label: '日本語',
  },
  {
    value: 'no',
    label: 'Norsk (bokmål)',
  },
  {
    value: 'nn',
    label: 'Norsk (nynorsk)',
  },
  {
    value: 'ce',
    label: 'Нохчийн',
  },
  {
    value: 'uz',
    label: 'Oʻzbekcha / Ўзбекча',
  },
  {
    value: 'pt',
    label: 'Português',
  },
  {
    value: 'kk',
    label: 'Қазақша / Qazaqşa / قازاقشا',
  },
  {
    value: 'ro',
    label: 'Română',
  },
  {
    value: 'ru',
    label: 'Русский',
  },
  {
    value: 'ceb',
    label: 'Sinugboanong Binisaya',
  },
  {
    value: 'sk',
    label: 'Slovenčina',
  },
  {
    value: 'sl',
    label: 'Slovenščina',
  },
  {
    value: 'sr',
    label: 'Српски / Srpski',
  },
  {
    value: 'sh',
    label: 'Srpskohrvatski / Српскохрватски',
  },
  {
    value: 'fi',
    label: 'Suomi',
  },
  {
    value: 'sv',
    label: 'Svenska',
  },
  {
    value: 'ta',
    label: 'தமிழ்',
  },
  {
    value: 'tt',
    label: 'Татарча / Tatarça',
  },
  {
    value: 'th',
    label: 'ภาษาไทย',
  },
  {
    value: 'tg',
    label: 'Тоҷикӣ',
  },
  {
    value: 'azb',
    label: 'تۆرکجه',
  },
  {
    value: 'tr',
    label: 'Türkçe',
  },
  {
    value: 'uk',
    label: 'Українська',
  },
  {
    value: 'ur',
    label: 'اردو',
  },
  {
    value: 'vi',
    label: 'Tiếng Việt',
  },
  {
    value: 'war',
    label: 'Winaray',
  },
  {
    value: 'zh',
    label: '中文',
  },
  {
    value: 'yue',
    label: '粵語',
  },
];

export const GoogleLanguageOptions = [
  {
    language_code: 'af',
    language_name: 'Afrikaans',
  },
  {
    language_code: 'ak',
    language_name: 'Akan',
  },
  {
    language_code: 'sq',
    language_name: 'Albanian',
  },
  {
    language_code: 'ws',
    language_name: 'Samoa',
  },
  {
    language_code: 'am',
    language_name: 'Amharic',
  },
  {
    language_code: 'ar',
    language_name: 'Arabic',
  },
  {
    language_code: 'hy',
    language_name: 'Armenian',
  },
  {
    language_code: 'az',
    language_name: 'Azerbaijani',
  },
  {
    language_code: 'eu',
    language_name: 'Basque',
  },
  {
    language_code: 'be',
    language_name: 'Belarusian',
  },
  {
    language_code: 'bem',
    language_name: 'Bemba',
  },
  {
    language_code: 'bn',
    language_name: 'Bengali',
  },
  {
    language_code: 'bh',
    language_name: 'Bihari',
  },
  {
    language_code: 'xx-bork',
    language_name: 'Bork, bork, bork!',
  },
  {
    language_code: 'bs',
    language_name: 'Bosnian',
  },
  {
    language_code: 'br',
    language_name: 'Breton',
  },
  {
    language_code: 'bg',
    language_name: 'Bulgarian',
  },
  {
    language_code: 'bt',
    language_name: 'Bhutanese',
  },
  {
    language_code: 'km',
    language_name: 'Cambodian',
  },
  {
    language_code: 'ca',
    language_name: 'Catalan',
  },
  {
    language_code: 'chr',
    language_name: 'Cherokee',
  },
  {
    language_code: 'ny',
    language_name: 'Chichewa',
  },
  {
    language_code: 'zh-cn',
    language_name: 'Chinese (Simplified)',
  },
  {
    language_code: 'zh-tw',
    language_name: 'Chinese (Traditional)',
  },
  {
    language_code: 'co',
    language_name: 'Corsican',
  },
  {
    language_code: 'hr',
    language_name: 'Croatian',
  },
  {
    language_code: 'cs',
    language_name: 'Czech',
  },
  {
    language_code: 'da',
    language_name: 'Danish',
  },
  {
    language_code: 'nl',
    language_name: 'Dutch',
  },
  {
    language_code: 'xx-elmer',
    language_name: 'Elmer Fudd',
  },
  {
    language_code: 'en',
    language_name: 'English',
  },
  {
    language_code: 'eo',
    language_name: 'Esperanto',
  },
  {
    language_code: 'et',
    language_name: 'Estonian',
  },
  {
    language_code: 'ee',
    language_name: 'Ewe',
  },
  {
    language_code: 'fo',
    language_name: 'Faroese',
  },
  {
    language_code: 'tl',
    language_name: 'Filipino',
  },
  {
    language_code: 'fi',
    language_name: 'Finnish',
  },
  {
    language_code: 'fr',
    language_name: 'French',
  },
  {
    language_code: 'fy',
    language_name: 'Frisian',
  },
  {
    language_code: 'gaa',
    language_name: 'Ga',
  },
  {
    language_code: 'gl',
    language_name: 'Galician',
  },
  {
    language_code: 'ka',
    language_name: 'Georgian',
  },
  {
    language_code: 'de',
    language_name: 'German',
  },
  {
    language_code: 'el',
    language_name: 'Greek',
  },
  {
    language_code: 'kl',
    language_name: 'Greenlandic',
  },
  {
    language_code: 'gn',
    language_name: 'Guarani',
  },
  {
    language_code: 'gu',
    language_name: 'Gujarati',
  },
  {
    language_code: 'xx-hacker',
    language_name: 'Hacker',
  },
  {
    language_code: 'ht',
    language_name: 'Haitian Creole',
  },
  {
    language_code: 'ha',
    language_name: 'Hausa',
  },
  {
    language_code: 'haw',
    language_name: 'Hawaiian',
  },
  {
    language_code: 'iw',
    language_name: 'Hebrew',
  },
  {
    language_code: 'hi',
    language_name: 'Hindi',
  },
  {
    language_code: 'hu',
    language_name: 'Hungarian',
  },
  {
    language_code: 'is',
    language_name: 'Icelandic',
  },
  {
    language_code: 'ig',
    language_name: 'Igbo',
  },
  {
    language_code: 'id',
    language_name: 'Indonesian',
  },
  {
    language_code: 'ia',
    language_name: 'Interlingua',
  },
  {
    language_code: 'ga',
    language_name: 'Irish',
  },
  {
    language_code: 'it',
    language_name: 'Italian',
  },
  {
    language_code: 'ja',
    language_name: 'Japanese',
  },
  {
    language_code: 'jw',
    language_name: 'Javanese',
  },
  {
    language_code: 'kn',
    language_name: 'Kannada',
  },
  {
    language_code: 'kk',
    language_name: 'Kazakh',
  },
  {
    language_code: 'rw',
    language_name: 'Kinyarwanda',
  },
  {
    language_code: 'rn',
    language_name: 'Kirundi',
  },
  {
    language_code: 'xx-klingon',
    language_name: 'Klingon',
  },
  {
    language_code: 'kg',
    language_name: 'Kongo',
  },
  {
    language_code: 'ko',
    language_name: 'Korean',
  },
  {
    language_code: 'kri',
    language_name: 'Krio (Sierra Leone)',
  },
  {
    language_code: 'ku',
    language_name: 'Kurdish',
  },
  {
    language_code: 'ckb',
    language_name: 'Kurdish (Soranî)',
  },
  {
    language_code: 'ky',
    language_name: 'Kyrgyz',
  },
  {
    language_code: 'lo',
    language_name: 'Laothian',
  },
  {
    language_code: 'la',
    language_name: 'Latin',
  },
  {
    language_code: 'lv',
    language_name: 'Latvian',
  },
  {
    language_code: 'ln',
    language_name: 'Lingala',
  },
  {
    language_code: 'lt',
    language_name: 'Lithuanian',
  },
  {
    language_code: 'loz',
    language_name: 'Lozi',
  },
  {
    language_code: 'lg',
    language_name: 'Luganda',
  },
  {
    language_code: 'ach',
    language_name: 'Luo',
  },
  {
    language_code: 'mk',
    language_name: 'Macedonian',
  },
  {
    language_code: 'mg',
    language_name: 'Malagasy',
  },
  {
    language_code: 'ms',
    language_name: 'Malay',
  },
  {
    language_code: 'ml',
    language_name: 'Malayalam',
  },
  {
    language_code: 'mt',
    language_name: 'Maltese',
  },
  {
    language_code: 'mv',
    language_name: 'Maldives',
  },
  {
    language_code: 'mi',
    language_name: 'Maori',
  },
  {
    language_code: 'mr',
    language_name: 'Marathi',
  },
  {
    language_code: 'mfe',
    language_name: 'Mauritian Creole',
  },
  {
    language_code: 'mo',
    language_name: 'Moldavian',
  },
  {
    language_code: 'mn',
    language_name: 'Mongolian',
  },
  {
    language_code: 'sr-me',
    language_name: 'Montenegrin',
  },
  {
    language_code: 'my',
    language_name: 'Myanmar',
  },
  {
    language_code: 'ne',
    language_name: 'Nepali',
  },
  {
    language_code: 'pcm',
    language_name: 'Nigerian Pidgin',
  },
  {
    language_code: 'nso',
    language_name: 'Northern Sotho',
  },
  {
    language_code: 'no',
    language_name: 'Norwegian',
  },
  {
    language_code: 'nn',
    language_name: 'Norwegian (Nynorsk)',
  },
  {
    language_code: 'oc',
    language_name: 'Occitan',
  },
  {
    language_code: 'or',
    language_name: 'Oriya',
  },
  {
    language_code: 'om',
    language_name: 'Oromo',
  },
  {
    language_code: 'ps',
    language_name: 'Pashto',
  },
  {
    language_code: 'fa',
    language_name: 'Persian',
  },
  {
    language_code: 'xx-pirate',
    language_name: 'Pirate',
  },
  {
    language_code: 'pl',
    language_name: 'Polish',
  },
  {
    language_code: 'pt',
    language_name: 'Portuguese',
  },
  {
    language_code: 'pt-br',
    language_name: 'Portuguese (Brazil)',
  },
  {
    language_code: 'pt-pt',
    language_name: 'Portuguese (Portugal)',
  },
  {
    language_code: 'pa',
    language_name: 'Punjabi',
  },
  {
    language_code: 'qu',
    language_name: 'Quechua',
  },
  {
    language_code: 'ro',
    language_name: 'Romanian',
  },
  {
    language_code: 'rm',
    language_name: 'Romansh',
  },
  {
    language_code: 'nyn',
    language_name: 'Runyakitara',
  },
  {
    language_code: 'ru',
    language_name: 'Russian',
  },
  {
    language_code: 'gd',
    language_name: 'Scots Gaelic',
  },
  {
    language_code: 'sr',
    language_name: 'Serbian',
  },
  {
    language_code: 'sh',
    language_name: 'Serbo-Croatian',
  },
  {
    language_code: 'st',
    language_name: 'Sesotho',
  },
  {
    language_code: 'tn',
    language_name: 'Setswana',
  },
  {
    language_code: 'crs',
    language_name: 'Seychellois Creole',
  },
  {
    language_code: 'sn',
    language_name: 'Shona',
  },
  {
    language_code: 'sd',
    language_name: 'Sindhi',
  },
  {
    language_code: 'si',
    language_name: 'Sinhalese',
  },
  {
    language_code: 'sk',
    language_name: 'Slovak',
  },
  {
    language_code: 'sl',
    language_name: 'Slovenian',
  },
  {
    language_code: 'so',
    language_name: 'Somali',
  },
  {
    language_code: 'es',
    language_name: 'Spanish',
  },
  {
    language_code: 'es-419',
    language_name: 'Spanish (Latin American)',
  },
  {
    language_code: 'su',
    language_name: 'Sundanese',
  },
  {
    language_code: 'sw',
    language_name: 'Swahili',
  },
  {
    language_code: 'sv',
    language_name: 'Swedish',
  },
  {
    language_code: 'tg',
    language_name: 'Tajik',
  },
  {
    language_code: 'ta',
    language_name: 'Tamil',
  },
  {
    language_code: 'tt',
    language_name: 'Tatar',
  },
  {
    language_code: 'te',
    language_name: 'Telugu',
  },
  {
    language_code: 'th',
    language_name: 'Thai',
  },
  {
    language_code: 'ti',
    language_name: 'Tigrinya',
  },
  {
    language_code: 'to',
    language_name: 'Tonga',
  },
  {
    language_code: 'lua',
    language_name: 'Tshiluba',
  },
  {
    language_code: 'tum',
    language_name: 'Tumbuka',
  },
  {
    language_code: 'tr',
    language_name: 'Turkish',
  },
  {
    language_code: 'tk',
    language_name: 'Turkmen',
  },
  {
    language_code: 'tw',
    language_name: 'Twi',
  },
  {
    language_code: 'ug',
    language_name: 'Uighur',
  },
  {
    language_code: 'uk',
    language_name: 'Ukrainian',
  },
  {
    language_code: 'ur',
    language_name: 'Urdu',
  },
  {
    language_code: 'uz',
    language_name: 'Uzbek',
  },
  {
    language_code: 'vu',
    language_name: 'Vanuatu',
  },
  {
    language_code: 'vi',
    language_name: 'Vietnamese',
  },
  {
    language_code: 'cy',
    language_name: 'Welsh',
  },
  {
    language_code: 'wo',
    language_name: 'Wolof',
  },
  {
    language_code: 'xh',
    language_name: 'Xhosa',
  },
  {
    language_code: 'yi',
    language_name: 'Yiddish',
  },
  {
    language_code: 'yo',
    language_name: 'Yoruba',
  },
  {
    language_code: 'zu',
    language_name: 'Zulu',
  },
].map((x) => ({ label: x.language_name, value: x.language_code }));

export const GoogleCountryOptions = [
  {
    country_code: 'af',
    country_name: 'Afghanistan',
  },
  {
    country_code: 'al',
    country_name: 'Albania',
  },
  {
    country_code: 'dz',
    country_name: 'Algeria',
  },
  {
    country_code: 'as',
    country_name: 'American Samoa',
  },
  {
    country_code: 'ad',
    country_name: 'Andorra',
  },
  {
    country_code: 'ao',
    country_name: 'Angola',
  },
  {
    country_code: 'ai',
    country_name: 'Anguilla',
  },
  {
    country_code: 'aq',
    country_name: 'Antarctica',
  },
  {
    country_code: 'ag',
    country_name: 'Antigua and Barbuda',
  },
  {
    country_code: 'ar',
    country_name: 'Argentina',
  },
  {
    country_code: 'am',
    country_name: 'Armenia',
  },
  {
    country_code: 'aw',
    country_name: 'Aruba',
  },
  {
    country_code: 'au',
    country_name: 'Australia',
  },
  {
    country_code: 'at',
    country_name: 'Austria',
  },
  {
    country_code: 'az',
    country_name: 'Azerbaijan',
  },
  {
    country_code: 'bs',
    country_name: 'Bahamas',
  },
  {
    country_code: 'bh',
    country_name: 'Bahrain',
  },
  {
    country_code: 'bd',
    country_name: 'Bangladesh',
  },
  {
    country_code: 'bb',
    country_name: 'Barbados',
  },
  {
    country_code: 'by',
    country_name: 'Belarus',
  },
  {
    country_code: 'be',
    country_name: 'Belgium',
  },
  {
    country_code: 'bz',
    country_name: 'Belize',
  },
  {
    country_code: 'bj',
    country_name: 'Benin',
  },
  {
    country_code: 'bm',
    country_name: 'Bermuda',
  },
  {
    country_code: 'bt',
    country_name: 'Bhutan',
  },
  {
    country_code: 'bo',
    country_name: 'Bolivia',
  },
  {
    country_code: 'ba',
    country_name: 'Bosnia and Herzegovina',
  },
  {
    country_code: 'bw',
    country_name: 'Botswana',
  },
  {
    country_code: 'bv',
    country_name: 'Bouvet Island',
  },
  {
    country_code: 'br',
    country_name: 'Brazil',
  },
  {
    country_code: 'io',
    country_name: 'British Indian Ocean Territory',
  },
  {
    country_code: 'bn',
    country_name: 'Brunei Darussalam',
  },
  {
    country_code: 'bg',
    country_name: 'Bulgaria',
  },
  {
    country_code: 'bf',
    country_name: 'Burkina Faso',
  },
  {
    country_code: 'bi',
    country_name: 'Burundi',
  },
  {
    country_code: 'kh',
    country_name: 'Cambodia',
  },
  {
    country_code: 'cm',
    country_name: 'Cameroon',
  },
  {
    country_code: 'ca',
    country_name: 'Canada',
  },
  {
    country_code: 'cv',
    country_name: 'Cape Verde',
  },
  {
    country_code: 'ky',
    country_name: 'Cayman Islands',
  },
  {
    country_code: 'cf',
    country_name: 'Central African Republic',
  },
  {
    country_code: 'td',
    country_name: 'Chad',
  },
  {
    country_code: 'cl',
    country_name: 'Chile',
  },
  {
    country_code: 'cn',
    country_name: 'China',
  },
  {
    country_code: 'cx',
    country_name: 'Christmas Island',
  },
  {
    country_code: 'cc',
    country_name: 'Cocos (Keeling) Islands',
  },
  {
    country_code: 'co',
    country_name: 'Colombia',
  },
  {
    country_code: 'km',
    country_name: 'Comoros',
  },
  {
    country_code: 'cg',
    country_name: 'Congo',
  },
  {
    country_code: 'cd',
    country_name: 'Congo, the Democratic Republic of the',
  },
  {
    country_code: 'ck',
    country_name: 'Cook Islands',
  },
  {
    country_code: 'cr',
    country_name: 'Costa Rica',
  },
  {
    country_code: 'ci',
    country_name: "Cote D'ivoire",
  },
  {
    country_code: 'hr',
    country_name: 'Croatia',
  },
  {
    country_code: 'cu',
    country_name: 'Cuba',
  },
  {
    country_code: 'cy',
    country_name: 'Cyprus',
  },
  {
    country_code: 'cz',
    country_name: 'Czech Republic',
  },
  {
    country_code: 'dk',
    country_name: 'Denmark',
  },
  {
    country_code: 'dj',
    country_name: 'Djibouti',
  },
  {
    country_code: 'dm',
    country_name: 'Dominica',
  },
  {
    country_code: 'do',
    country_name: 'Dominican Republic',
  },
  {
    country_code: 'ec',
    country_name: 'Ecuador',
  },
  {
    country_code: 'eg',
    country_name: 'Egypt',
  },
  {
    country_code: 'sv',
    country_name: 'El Salvador',
  },
  {
    country_code: 'gq',
    country_name: 'Equatorial Guinea',
  },
  {
    country_code: 'er',
    country_name: 'Eritrea',
  },
  {
    country_code: 'ee',
    country_name: 'Estonia',
  },
  {
    country_code: 'et',
    country_name: 'Ethiopia',
  },
  {
    country_code: 'fk',
    country_name: 'Falkland Islands (Malvinas)',
  },
  {
    country_code: 'fo',
    country_name: 'Faroe Islands',
  },
  {
    country_code: 'fj',
    country_name: 'Fiji',
  },
  {
    country_code: 'fi',
    country_name: 'Finland',
  },
  {
    country_code: 'fr',
    country_name: 'France',
  },
  {
    country_code: 'gf',
    country_name: 'French Guiana',
  },
  {
    country_code: 'pf',
    country_name: 'French Polynesia',
  },
  {
    country_code: 'tf',
    country_name: 'French Southern Territories',
  },
  {
    country_code: 'ga',
    country_name: 'Gabon',
  },
  {
    country_code: 'gm',
    country_name: 'Gambia',
  },
  {
    country_code: 'ge',
    country_name: 'Georgia',
  },
  {
    country_code: 'de',
    country_name: 'Germany',
  },
  {
    country_code: 'gh',
    country_name: 'Ghana',
  },
  {
    country_code: 'gi',
    country_name: 'Gibraltar',
  },
  {
    country_code: 'gr',
    country_name: 'Greece',
  },
  {
    country_code: 'gl',
    country_name: 'Greenland',
  },
  {
    country_code: 'gd',
    country_name: 'Grenada',
  },
  {
    country_code: 'gp',
    country_name: 'Guadeloupe',
  },
  {
    country_code: 'gu',
    country_name: 'Guam',
  },
  {
    country_code: 'gt',
    country_name: 'Guatemala',
  },
  {
    country_code: 'gn',
    country_name: 'Guinea',
  },
  {
    country_code: 'gw',
    country_name: 'Guinea-Bissau',
  },
  {
    country_code: 'gy',
    country_name: 'Guyana',
  },
  {
    country_code: 'ht',
    country_name: 'Haiti',
  },
  {
    country_code: 'hm',
    country_name: 'Heard Island and Mcdonald Islands',
  },
  {
    country_code: 'va',
    country_name: 'Holy See (Vatican City State)',
  },
  {
    country_code: 'hn',
    country_name: 'Honduras',
  },
  {
    country_code: 'hk',
    country_name: 'Hong Kong',
  },
  {
    country_code: 'hu',
    country_name: 'Hungary',
  },
  {
    country_code: 'is',
    country_name: 'Iceland',
  },
  {
    country_code: 'in',
    country_name: 'India',
  },
  {
    country_code: 'id',
    country_name: 'Indonesia',
  },
  {
    country_code: 'ir',
    country_name: 'Iran, Islamic Republic of',
  },
  {
    country_code: 'iq',
    country_name: 'Iraq',
  },
  {
    country_code: 'ie',
    country_name: 'Ireland',
  },
  {
    country_code: 'il',
    country_name: 'Israel',
  },
  {
    country_code: 'it',
    country_name: 'Italy',
  },
  {
    country_code: 'jm',
    country_name: 'Jamaica',
  },
  {
    country_code: 'jp',
    country_name: 'Japan',
  },
  {
    country_code: 'jo',
    country_name: 'Jordan',
  },
  {
    country_code: 'kz',
    country_name: 'Kazakhstan',
  },
  {
    country_code: 'ke',
    country_name: 'Kenya',
  },
  {
    country_code: 'ki',
    country_name: 'Kiribati',
  },
  {
    country_code: 'kp',
    country_name: "Korea, Democratic People's Republic of",
  },
  {
    country_code: 'kr',
    country_name: 'Korea, Republic of',
  },
  {
    country_code: 'kw',
    country_name: 'Kuwait',
  },
  {
    country_code: 'kg',
    country_name: 'Kyrgyzstan',
  },
  {
    country_code: 'la',
    country_name: "Lao People's Democratic Republic",
  },
  {
    country_code: 'lv',
    country_name: 'Latvia',
  },
  {
    country_code: 'lb',
    country_name: 'Lebanon',
  },
  {
    country_code: 'ls',
    country_name: 'Lesotho',
  },
  {
    country_code: 'lr',
    country_name: 'Liberia',
  },
  {
    country_code: 'ly',
    country_name: 'Libyan Arab Jamahiriya',
  },
  {
    country_code: 'li',
    country_name: 'Liechtenstein',
  },
  {
    country_code: 'lt',
    country_name: 'Lithuania',
  },
  {
    country_code: 'lu',
    country_name: 'Luxembourg',
  },
  {
    country_code: 'mo',
    country_name: 'Macao',
  },
  {
    country_code: 'mk',
    country_name: 'Macedonia, the Former Yugosalv Republic of',
  },
  {
    country_code: 'mg',
    country_name: 'Madagascar',
  },
  {
    country_code: 'mw',
    country_name: 'Malawi',
  },
  {
    country_code: 'my',
    country_name: 'Malaysia',
  },
  {
    country_code: 'mv',
    country_name: 'Maldives',
  },
  {
    country_code: 'ml',
    country_name: 'Mali',
  },
  {
    country_code: 'mt',
    country_name: 'Malta',
  },
  {
    country_code: 'mh',
    country_name: 'Marshall Islands',
  },
  {
    country_code: 'mq',
    country_name: 'Martinique',
  },
  {
    country_code: 'mr',
    country_name: 'Mauritania',
  },
  {
    country_code: 'mu',
    country_name: 'Mauritius',
  },
  {
    country_code: 'yt',
    country_name: 'Mayotte',
  },
  {
    country_code: 'mx',
    country_name: 'Mexico',
  },
  {
    country_code: 'fm',
    country_name: 'Micronesia, Federated States of',
  },
  {
    country_code: 'md',
    country_name: 'Moldova, Republic of',
  },
  {
    country_code: 'mc',
    country_name: 'Monaco',
  },
  {
    country_code: 'mn',
    country_name: 'Mongolia',
  },
  {
    country_code: 'ms',
    country_name: 'Montserrat',
  },
  {
    country_code: 'ma',
    country_name: 'Morocco',
  },
  {
    country_code: 'mz',
    country_name: 'Mozambique',
  },
  {
    country_code: 'mm',
    country_name: 'Myanmar',
  },
  {
    country_code: 'na',
    country_name: 'Namibia',
  },
  {
    country_code: 'nr',
    country_name: 'Nauru',
  },
  {
    country_code: 'np',
    country_name: 'Nepal',
  },
  {
    country_code: 'nl',
    country_name: 'Netherlands',
  },
  {
    country_code: 'an',
    country_name: 'Netherlands Antilles',
  },
  {
    country_code: 'nc',
    country_name: 'New Caledonia',
  },
  {
    country_code: 'nz',
    country_name: 'New Zealand',
  },
  {
    country_code: 'ni',
    country_name: 'Nicaragua',
  },
  {
    country_code: 'ne',
    country_name: 'Niger',
  },
  {
    country_code: 'ng',
    country_name: 'Nigeria',
  },
  {
    country_code: 'nu',
    country_name: 'Niue',
  },
  {
    country_code: 'nf',
    country_name: 'Norfolk Island',
  },
  {
    country_code: 'mp',
    country_name: 'Northern Mariana Islands',
  },
  {
    country_code: 'no',
    country_name: 'Norway',
  },
  {
    country_code: 'om',
    country_name: 'Oman',
  },
  {
    country_code: 'pk',
    country_name: 'Pakistan',
  },
  {
    country_code: 'pw',
    country_name: 'Palau',
  },
  {
    country_code: 'ps',
    country_name: 'Palestinian Territory, Occupied',
  },
  {
    country_code: 'pa',
    country_name: 'Panama',
  },
  {
    country_code: 'pg',
    country_name: 'Papua New Guinea',
  },
  {
    country_code: 'py',
    country_name: 'Paraguay',
  },
  {
    country_code: 'pe',
    country_name: 'Peru',
  },
  {
    country_code: 'ph',
    country_name: 'Philippines',
  },
  {
    country_code: 'pn',
    country_name: 'Pitcairn',
  },
  {
    country_code: 'pl',
    country_name: 'Poland',
  },
  {
    country_code: 'pt',
    country_name: 'Portugal',
  },
  {
    country_code: 'pr',
    country_name: 'Puerto Rico',
  },
  {
    country_code: 'qa',
    country_name: 'Qatar',
  },
  {
    country_code: 're',
    country_name: 'Reunion',
  },
  {
    country_code: 'ro',
    country_name: 'Romania',
  },
  {
    country_code: 'ru',
    country_name: 'Russian Federation',
  },
  {
    country_code: 'rw',
    country_name: 'Rwanda',
  },
  {
    country_code: 'sh',
    country_name: 'Saint Helena',
  },
  {
    country_code: 'kn',
    country_name: 'Saint Kitts and Nevis',
  },
  {
    country_code: 'lc',
    country_name: 'Saint Lucia',
  },
  {
    country_code: 'pm',
    country_name: 'Saint Pierre and Miquelon',
  },
  {
    country_code: 'vc',
    country_name: 'Saint Vincent and the Grenadines',
  },
  {
    country_code: 'ws',
    country_name: 'Samoa',
  },
  {
    country_code: 'sm',
    country_name: 'San Marino',
  },
  {
    country_code: 'st',
    country_name: 'Sao Tome and Principe',
  },
  {
    country_code: 'sa',
    country_name: 'Saudi Arabia',
  },
  {
    country_code: 'sn',
    country_name: 'Senegal',
  },
  {
    country_code: 'rs',
    country_name: 'Serbia and Montenegro',
  },
  {
    country_code: 'sc',
    country_name: 'Seychelles',
  },
  {
    country_code: 'sl',
    country_name: 'Sierra Leone',
  },
  {
    country_code: 'sg',
    country_name: 'Singapore',
  },
  {
    country_code: 'sk',
    country_name: 'Slovakia',
  },
  {
    country_code: 'si',
    country_name: 'Slovenia',
  },
  {
    country_code: 'sb',
    country_name: 'Solomon Islands',
  },
  {
    country_code: 'so',
    country_name: 'Somalia',
  },
  {
    country_code: 'za',
    country_name: 'South Africa',
  },
  {
    country_code: 'gs',
    country_name: 'South Georgia and the South Sandwich Islands',
  },
  {
    country_code: 'es',
    country_name: 'Spain',
  },
  {
    country_code: 'lk',
    country_name: 'Sri Lanka',
  },
  {
    country_code: 'sd',
    country_name: 'Sudan',
  },
  {
    country_code: 'sr',
    country_name: 'Suriname',
  },
  {
    country_code: 'sj',
    country_name: 'Svalbard and Jan Mayen',
  },
  {
    country_code: 'sz',
    country_name: 'Swaziland',
  },
  {
    country_code: 'se',
    country_name: 'Sweden',
  },
  {
    country_code: 'ch',
    country_name: 'Switzerland',
  },
  {
    country_code: 'sy',
    country_name: 'Syrian Arab Republic',
  },
  {
    country_code: 'tw',
    country_name: 'Taiwan, Province of China',
  },
  {
    country_code: 'tj',
    country_name: 'Tajikistan',
  },
  {
    country_code: 'tz',
    country_name: 'Tanzania, United Republic of',
  },
  {
    country_code: 'th',
    country_name: 'Thailand',
  },
  {
    country_code: 'tl',
    country_name: 'Timor-Leste',
  },
  {
    country_code: 'tg',
    country_name: 'Togo',
  },
  {
    country_code: 'tk',
    country_name: 'Tokelau',
  },
  {
    country_code: 'to',
    country_name: 'Tonga',
  },
  {
    country_code: 'tt',
    country_name: 'Trinidad and Tobago',
  },
  {
    country_code: 'tn',
    country_name: 'Tunisia',
  },
  {
    country_code: 'tr',
    country_name: 'Turkiye',
  },
  {
    country_code: 'tm',
    country_name: 'Turkmenistan',
  },
  {
    country_code: 'tc',
    country_name: 'Turks and Caicos Islands',
  },
  {
    country_code: 'tv',
    country_name: 'Tuvalu',
  },
  {
    country_code: 'ug',
    country_name: 'Uganda',
  },
  {
    country_code: 'ua',
    country_name: 'Ukraine',
  },
  {
    country_code: 'ae',
    country_name: 'United Arab Emirates',
  },
  {
    country_code: 'uk',
    country_name: 'United Kingdom',
  },
  {
    country_code: 'gb',
    country_name: 'United Kingdom',
  },
  {
    country_code: 'us',
    country_name: 'United States',
  },
  {
    country_code: 'um',
    country_name: 'United States Minor Outlying Islands',
  },
  {
    country_code: 'uy',
    country_name: 'Uruguay',
  },
  {
    country_code: 'uz',
    country_name: 'Uzbekistan',
  },
  {
    country_code: 'vu',
    country_name: 'Vanuatu',
  },
  {
    country_code: 've',
    country_name: 'Venezuela',
  },
  {
    country_code: 'vn',
    country_name: 'Viet Nam',
  },
  {
    country_code: 'vg',
    country_name: 'Virgin Islands, British',
  },
  {
    country_code: 'vi',
    country_name: 'Virgin Islands, U.S.',
  },
  {
    country_code: 'wf',
    country_name: 'Wallis and Futuna',
  },
  {
    country_code: 'eh',
    country_name: 'Western Sahara',
  },
  {
    country_code: 'ye',
    country_name: 'Yemen',
  },
  {
    country_code: 'zm',
    country_name: 'Zambia',
  },
  {
    country_code: 'zw',
    country_name: 'Zimbabwe',
  },
].map((x) => ({ label: x.country_name, value: x.country_code }));

export const BingCountryOptions = [
  { label: 'Argentina AR', value: 'AR' },
  { label: 'Australia AU', value: 'AU' },
  { label: 'Austria AT', value: 'AT' },
  { label: 'Belgium BE', value: 'BE' },
  { label: 'Brazil BR', value: 'BR' },
  { label: 'Canada CA', value: 'CA' },
  { label: 'Chile CL', value: 'CL' },
  { label: 'Denmark DK', value: 'DK' },
  { label: 'Finland FI', value: 'FI' },
  { label: 'France FR', value: 'FR' },
  { label: 'Germany DE', value: 'DE' },
  { label: 'Hong Kong SAR HK', value: 'HK' },
  { label: 'India IN', value: 'IN' },
  { label: 'Indonesia ID', value: 'ID' },
  { label: 'Italy IT', value: 'IT' },
  { label: 'Japan JP', value: 'JP' },
  { label: 'Korea KR', value: 'KR' },
  { label: 'Malaysia MY', value: 'MY' },
  { label: 'Mexico MX', value: 'MX' },
  { label: 'Netherlands NL', value: 'NL' },
  { label: 'New Zealand NZ', value: 'NZ' },
  { label: 'Norway NO', value: 'NO' },
  { label: "People's Republic of China CN", value: 'CN' },
  { label: 'Poland PL', value: 'PL' },
  { label: 'Portugal PT', value: 'PT' },
  { label: 'Republic of the Philippines PH', value: 'PH' },
  { label: 'Russia RU', value: 'RU' },
  { label: 'Saudi Arabia SA', value: 'SA' },
  { label: 'South Africa ZA', value: 'ZA' },
  { label: 'Spain ES', value: 'ES' },
  { label: 'Sweden SE', value: 'SE' },
  { label: 'Switzerland CH', value: 'CH' },
  { label: 'Taiwan TW', value: 'TW' },
  { label: 'Türkiye TR', value: 'TR' },
  { label: 'United Kingdom GB', value: 'GB' },
  { label: 'United States US', value: 'US' },
];

export const BingLanguageOptions = [
  { label: 'Arabic ar', value: 'ar' },
  { label: 'Basque eu', value: 'eu' },
  { label: 'Bengali bn', value: 'bn' },
  { label: 'Bulgarian bg', value: 'bg' },
  { label: 'Catalan ca', value: 'ca' },
  { label: 'Chinese (Simplified) zh-hans', value: 'ns' },
  { label: 'Chinese (Traditional) zh-hant', value: 'nt' },
  { label: 'Croatian hr', value: 'hr' },
  { label: 'Czech cs', value: 'cs' },
  { label: 'Danish da', value: 'da' },
  { label: 'Dutch nl', value: 'nl' },
  { label: 'English en', value: 'en' },
  { label: 'English-United Kingdom en-gb', value: 'gb' },
  { label: 'Estonian et', value: 'et' },
  { label: 'Finnish fi', value: 'fi' },
  { label: 'French fr', value: 'fr' },
  { label: 'Galician gl', value: 'gl' },
  { label: 'German de', value: 'de' },
  { label: 'Gujarati gu', value: 'gu' },
  { label: 'Hebrew he', value: 'he' },
  { label: 'Hindi hi', value: 'hi' },
  { label: 'Hungarian hu', value: 'hu' },
  { label: 'Icelandic is', value: 'is' },
  { label: 'Italian it', value: 'it' },
  { label: 'Japanese jp', value: 'jp' },
  { label: 'Kannada kn', value: 'kn' },
  { label: 'Korean ko', value: 'ko' },
  { label: 'Latvian lv', value: 'lv' },
  { label: 'Lithuanian lt', value: 'lt' },
  { label: 'Malay ms', value: 'ms' },
  { label: 'Malayalam ml', value: 'ml' },
  { label: 'Marathi mr', value: 'mr' },
  { label: 'Norwegian (Bokmål) nb', value: 'nb' },
  { label: 'Polish pl', value: 'pl' },
  { label: 'Portuguese (Brazil) pt-br', value: 'br' },
  { label: 'Portuguese (Portugal) pt-pt', value: 'pt' },
  { label: 'Punjabi pa', value: 'pa' },
  { label: 'Romanian ro', value: 'ro' },
  { label: 'Russian ru', value: 'ru' },
  { label: 'Serbian (Cyrylic) sr', value: 'sr' },
  { label: 'Slovak sk', value: 'sk' },
  { label: 'Slovenian sl', value: 'sl' },
  { label: 'Spanish es', value: 'es' },
  { label: 'Swedish sv', value: 'sv' },
  { label: 'Tamil ta', value: 'ta' },
  { label: 'Telugu te', value: 'te' },
  { label: 'Thai th', value: 'th' },
  { label: 'Turkish tr', value: 'tr' },
  { label: 'Ukrainian uk', value: 'uk' },
  { label: 'Vietnamese vi', value: 'vi' },
];

export const DeepLSourceLangOptions = [
  { label: 'Arabic [1]', value: 'AR' },
  { label: 'Bulgarian', value: 'BG' },
  { label: 'Czech', value: 'CS' },
  { label: 'Danish', value: 'DA' },
  { label: 'German', value: 'DE' },
  { label: 'Greek', value: 'EL' },
  { label: 'English', value: 'EN' },
  { label: 'Spanish', value: 'ES' },
  { label: 'Estonian', value: 'ET' },
  { label: 'Finnish', value: 'FI' },
  { label: 'French', value: 'FR' },
  { label: 'Hungarian', value: 'HU' },
  { label: 'Indonesian', value: 'ID' },
  { label: 'Italian', value: 'IT' },
  { label: 'Japanese', value: 'JA' },
  { label: 'Korean', value: 'KO' },
  { label: 'Lithuanian', value: 'LT' },
  { label: 'Latvian', value: 'LV' },
  { label: 'Norwegian Bokmål', value: 'NB' },
  { label: 'Dutch', value: 'NL' },
  { label: 'Polish', value: 'PL' },
  { label: 'Portuguese (all Portuguese varieties mixed)', value: 'PT' },
  { label: 'Romanian', value: 'RO' },
  { label: 'Russian', value: 'RU' },
  { label: 'Slovak', value: 'SK' },
  { label: 'Slovenian', value: 'SL' },
  { label: 'Swedish', value: 'SV' },
  { label: 'Turkish', value: 'TR' },
  { label: 'Ukrainian', value: 'UK' },
  { label: 'Chinese', value: 'ZH' },
];
export const DeepLTargetLangOptions = [
  { label: 'Arabic [1]', value: 'AR' },
  { label: 'Bulgarian', value: 'BG' },
  { label: 'Czech', value: 'CS' },
  { label: 'Danish', value: 'DA' },
  { label: 'German', value: 'DE' },
  { label: 'Greek', value: 'EL' },
  { label: 'English (British)', value: 'EN-GB' },
  { label: 'English (American)', value: 'EN-US' },
  { label: 'Spanish', value: 'ES' },
  { label: 'Estonian', value: 'ET' },
  { label: 'Finnish', value: 'FI' },
  { label: 'French', value: 'FR' },
  { label: 'Hungarian', value: 'HU' },
  { label: 'Indonesian', value: 'ID' },
  { label: 'Italian', value: 'IT' },
  { label: 'Japanese', value: 'JA' },
  { label: 'Korean', value: 'KO' },
  { label: 'Lithuanian', value: 'LT' },
  { label: 'Latvian', value: 'LV' },
  { label: 'Norwegian Bokmål', value: 'NB' },
  { label: 'Dutch', value: 'NL' },
  { label: 'Polish', value: 'PL' },
  { label: 'Portuguese (Brazilian)', value: 'PT-BR' },
  {
    label:
      'Portuguese (all Portuguese varieties excluding Brazilian Portuguese)',
    value: 'PT-PT',
  },
  { label: 'Romanian', value: 'RO' },
  { label: 'Russian', value: 'RU' },
  { label: 'Slovak', value: 'SK' },
  { label: 'Slovenian', value: 'SL' },
  { label: 'Swedish', value: 'SV' },
  { label: 'Turkish', value: 'TR' },
  { label: 'Ukrainian', value: 'UK' },
  { label: 'Chinese (simplified)', value: 'ZH' },
];

export const BaiduFanyiDomainOptions = [
  'it',
  'finance',
  'machinery',
  'senimed',
  'novel',
  'academic',
  'aerospace',
  'wiki',
  'news',
  'law',
  'contract',
];

export const BaiduFanyiSourceLangOptions = [
  'auto',
  'zh',
  'en',
  'yue',
  'wyw',
  'jp',
  'kor',
  'fra',
  'spa',
  'th',
  'ara',
  'ru',
  'pt',
  'de',
  'it',
  'el',
  'nl',
  'pl',
  'bul',
  'est',
  'dan',
  'fin',
  'cs',
  'rom',
  'slo',
  'swe',
  'hu',
  'cht',
  'vie',
];

export const QWeatherLangOptions = [
  'zh',
  'zh-hant',
  'en',
  'de',
  'es',
  'fr',
  'it',
  'ja',
  'ko',
  'ru',
  'hi',
  'th',
  'ar',
  'pt',
  'bn',
  'ms',
  'nl',
  'el',
  'la',
  'sv',
  'id',
  'pl',
  'tr',
  'cs',
  'et',
  'vi',
  'fil',
  'fi',
  'he',
  'is',
  'nb',
];

export const QWeatherTypeOptions = ['weather', 'indices', 'airquality'];

export const QWeatherUserTypeOptions = ['free', 'paid'];

export const QWeatherTimePeriodOptions = [
  'now',
  '3d',
  '7d',
  '10d',
  '15d',
  '30d',
];

export const ExeSQLOptions = ['mysql', 'postgresql', 'mariadb', 'mssql'].map(
  (x) => ({
    label: upperFirst(x),
    value: x,
  }),
);

export const SwitchElseTo = 'end_cpn_id';

export const SwitchOperatorOptions = [
  { value: '=', label: 'equal' },
  { value: '≠', label: 'notEqual' },
  { value: '>', label: 'gt' },
  { value: '≥', label: 'ge' },
  { value: '<', label: 'lt' },
  { value: '≤', label: 'le' },
  { value: 'contains', label: 'contains' },
  { value: 'not contains', label: 'notContains' },
  { value: 'start with', label: 'startWith' },
  { value: 'end with', label: 'endWith' },
  { value: 'empty', label: 'empty' },
  { value: 'not empty', label: 'notEmpty' },
];

export const SwitchLogicOperatorOptions = ['and', 'or'];

export const WenCaiQueryTypeOptions = [
  'stock',
  'zhishu',
  'fund',
  'hkstock',
  'usstock',
  'threeboard',
  'conbond',
  'insurance',
  'futures',
  'lccp',
  'foreign_exchange',
];

export const Jin10TypeOptions = ['flash', 'calendar', 'symbols', 'news'];
export const Jin10FlashTypeOptions = new Array(5)
  .fill(1)
  .map((x, idx) => (idx + 1).toString());
export const Jin10CalendarTypeOptions = ['cj', 'qh', 'hk', 'us'];
export const Jin10CalendarDatashapeOptions = ['data', 'event', 'holiday'];
export const Jin10SymbolsTypeOptions = ['GOODS', 'FOREX', 'FUTURE', 'CRYPTO'];
export const Jin10SymbolsDatatypeOptions = ['symbols', 'quotes'];
export const TuShareSrcOptions = [
  'sina',
  'wallstreetcn',
  '10jqka',
  'eastmoney',
  'yuncaijing',
  'fenghuang',
  'jinrongjie',
];
export const CrawlerResultOptions = ['markdown', 'html', 'content'];

export enum BeginQueryType {
  Line = 'line',
  Paragraph = 'paragraph',
  Options = 'options',
  File = 'file',
  Integer = 'integer',
  Boolean = 'boolean',
  KnowledgeBases = 'kb',
}

export const BeginQueryTypeIconMap = {
  [BeginQueryType.Line]: TextCursorInput,
  [BeginQueryType.Paragraph]: WrapText,
  [BeginQueryType.Options]: OptionIcon,
  [BeginQueryType.File]: CloudUpload,
  [BeginQueryType.Integer]: ListOrdered,
  [BeginQueryType.Boolean]: ToggleLeft,
  [BeginQueryType.KnowledgeBases]: Database,
};

export const NoDebugOperatorsList = [
  Operator.Begin,
  Operator.Answer,
  Operator.Concentrator,
  Operator.Template,
  Operator.Message,
  Operator.RewriteQuestion,
  Operator.Switch,
  Operator.Iteration,
];
