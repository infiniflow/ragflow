import { ReactComponent as ArxivIcon } from '@/assets/svg/arxiv.svg';
import { ReactComponent as BaiduIcon } from '@/assets/svg/baidu.svg';
import { ReactComponent as DuckIcon } from '@/assets/svg/duck.svg';
import { ReactComponent as KeywordIcon } from '@/assets/svg/keyword.svg';
import { ReactComponent as PubMedIcon } from '@/assets/svg/pubmed.svg';
import { ReactComponent as WikipediaIcon } from '@/assets/svg/wikipedia.svg';

import { variableEnabledFieldMap } from '@/constants/chat';
import i18n from '@/locales/config';

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
  SlidersOutlined,
} from '@ant-design/icons';

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
  Arxiv = 'Arxiv',
}

export const operatorIconMap = {
  [Operator.Retrieval]: RocketOutlined,
  [Operator.Generate]: MergeCellsOutlined,
  [Operator.Answer]: SendOutlined,
  [Operator.Begin]: SlidersOutlined,
  [Operator.Categorize]: DatabaseOutlined,
  [Operator.Message]: MessageOutlined,
  [Operator.Relevant]: BranchesOutlined,
  [Operator.RewriteQuestion]: FormOutlined,
  [Operator.KeywordExtract]: KeywordIcon,
  [Operator.DuckDuckGo]: DuckIcon,
  [Operator.Baidu]: BaiduIcon,
  [Operator.Wikipedia]: WikipediaIcon,
  [Operator.PubMed]: PubMedIcon,
  [Operator.Arxiv]: ArxivIcon,
};

export const operatorMap = {
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
    color: 'white',
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
    color: 'white',
    width: 70,
    height: 70,
    fontSize: 12,
    iconFontSize: 16,
  },
  [Operator.RewriteQuestion]: {
    backgroundColor: '#f8c7f8',
    color: 'white',
    width: 70,
    height: 70,
    fontSize: 12,
    iconFontSize: 16,
  },
  [Operator.KeywordExtract]: {
    width: 70,
    height: 70,
    backgroundColor: '#0f0e0f',
    color: '#e1dcdc',
    fontSize: 12,
    iconWidth: 16,
    // iconFontSize: 16,
  },
  [Operator.DuckDuckGo]: {
    backgroundColor: '#e7e389',
    color: '#aea00c',
  },
  [Operator.Baidu]: {},
  [Operator.Wikipedia]: {
    backgroundColor: '#dee0e2',
  },
  [Operator.PubMed]: {
    backgroundColor: '#a2ccf0',
  },
  [Operator.Arxiv]: {
    width: 70,
    height: 70,
    fontSize: 12,
    iconWidth: 16,
    iconFontSize: 16,
    moreIconColor: 'white',
    backgroundColor: '#b31b1b',
    color: 'white',
  },
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
    name: Operator.Relevant,
  },
  {
    name: Operator.RewriteQuestion,
  },
  {
    name: Operator.KeywordExtract,
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
    name: Operator.Arxiv,
  },
];

export const initialRetrievalValues = {
  similarity_threshold: 0.2,
  keywords_similarity_weight: 0.3,
  top_n: 8,
};

export const initialBeginValues = {
  prologue: `Hi! I'm your assistant, what can I do for you?`,
};

export const variableCheckBoxFieldMap = Object.keys(
  variableEnabledFieldMap,
).reduce<Record<string, boolean>>((pre, cur) => {
  pre[cur] = true;
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
  loop: 1,
};

export const initialRelevantValues = {
  ...initialLlmBaseValues,
};

export const initialCategorizeValues = {
  ...initialLlmBaseValues,
  category_description: {},
};

export const initialMessageValues = {
  messages: [],
};

export const initialKeywordExtractValues = {
  ...initialLlmBaseValues,
  top_n: 1,
};
export const initialDuckValues = {
  top_n: 10,
  channel: Channel.Text,
};

export const initialBaiduValues = {
  top_n: 10,
};

export const initialWikipediaValues = {
  top_n: 10,
  language: 'en',
};

export const initialPubMedValues = {
  top_n: 10,
  email: '',
};

export const initialArxivValues = {
  top_n: 10,
  sort_by: 'relevance',
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
    Operator.Generate,
    Operator.RewriteQuestion,
    Operator.Categorize,
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
  [Operator.Arxiv]: [Operator.Begin, Operator.Retrieval],
};

export const NodeMap = {
  [Operator.Begin]: 'beginNode',
  [Operator.Categorize]: 'categorizeNode',
  [Operator.Retrieval]: 'ragNode',
  [Operator.Generate]: 'ragNode',
  [Operator.Answer]: 'ragNode',
  [Operator.Message]: 'ragNode',
  [Operator.Relevant]: 'relevantNode',
  [Operator.RewriteQuestion]: 'ragNode',
  [Operator.KeywordExtract]: 'ragNode',
  [Operator.DuckDuckGo]: 'ragNode',
  [Operator.Baidu]: 'ragNode',
  [Operator.Wikipedia]: 'ragNode',
  [Operator.PubMed]: 'ragNode',
  [Operator.Arxiv]: 'ragNode',
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
