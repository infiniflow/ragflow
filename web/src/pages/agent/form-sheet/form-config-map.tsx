import { Operator } from '../constant';
import AgentForm from '../form/agent-form';
import AkShareForm from '../form/akshare-form';
import ArXivForm from '../form/arxiv-form';
import BaiduFanyiForm from '../form/baidu-fanyi-form';
import BaiduForm from '../form/baidu-form';
import BeginForm from '../form/begin-form';
import BingForm from '../form/bing-form';
import CategorizeForm from '../form/categorize-form';
import CodeForm from '../form/code-form';
import CrawlerForm from '../form/crawler-form';
import DeepLForm from '../form/deepl-form';
import DuckDuckGoForm from '../form/duckduckgo-form';
import EmailForm from '../form/email-form';
import ExeSQLForm from '../form/exesql-form';
import GithubForm from '../form/github-form';
import GoogleForm from '../form/google-form';
import GoogleScholarForm from '../form/google-scholar-form';
import InvokeForm from '../form/invoke-form';
import IterationForm from '../form/iteration-form';
import IterationStartForm from '../form/iteration-start-from';
import Jin10Form from '../form/jin10-form';
import KeywordExtractForm from '../form/keyword-extract-form';
import MessageForm from '../form/message-form';
import PubMedForm from '../form/pubmed-form';
import QWeatherForm from '../form/qweather-form';
import RelevantForm from '../form/relevant-form';
import RetrievalForm from '../form/retrieval-form/next';
import RewriteQuestionForm from '../form/rewrite-question-form';
import StringTransformForm from '../form/string-transform-form';
import SwitchForm from '../form/switch-form';
import TavilyExtractForm from '../form/tavily-extract-form';
import TavilyForm from '../form/tavily-form';
import ToolForm from '../form/tool-form';
import TuShareForm from '../form/tushare-form';
import UserFillUpForm from '../form/user-fill-up-form';
import WenCaiForm from '../form/wencai-form';
import WikipediaForm from '../form/wikipedia-form';
import YahooFinanceForm from '../form/yahoo-finance-form';

export const FormConfigMap = {
  [Operator.Begin]: {
    component: BeginForm,
  },
  [Operator.Retrieval]: {
    component: RetrievalForm,
  },
  [Operator.Categorize]: {
    component: CategorizeForm,
  },
  [Operator.Message]: {
    component: MessageForm,
  },
  [Operator.Relevant]: {
    component: RelevantForm,
  },
  [Operator.RewriteQuestion]: {
    component: RewriteQuestionForm,
  },
  [Operator.Code]: {
    component: CodeForm,
  },
  [Operator.WaitingDialogue]: {
    component: CodeForm,
  },
  [Operator.Agent]: {
    component: AgentForm,
  },
  [Operator.Baidu]: {
    component: BaiduForm,
  },
  [Operator.DuckDuckGo]: {
    component: DuckDuckGoForm,
  },
  [Operator.KeywordExtract]: {
    component: KeywordExtractForm,
  },
  [Operator.Wikipedia]: {
    component: WikipediaForm,
  },
  [Operator.PubMed]: {
    component: PubMedForm,
  },
  [Operator.ArXiv]: {
    component: ArXivForm,
  },
  [Operator.Google]: {
    component: GoogleForm,
  },
  [Operator.Bing]: {
    component: BingForm,
  },
  [Operator.GoogleScholar]: {
    component: GoogleScholarForm,
  },
  [Operator.DeepL]: {
    component: DeepLForm,
  },
  [Operator.GitHub]: {
    component: GithubForm,
  },
  [Operator.BaiduFanyi]: {
    component: BaiduFanyiForm,
  },
  [Operator.QWeather]: {
    component: QWeatherForm,
  },
  [Operator.ExeSQL]: {
    component: ExeSQLForm,
  },
  [Operator.Switch]: {
    component: SwitchForm,
  },
  [Operator.WenCai]: {
    component: WenCaiForm,
  },
  [Operator.AkShare]: {
    component: AkShareForm,
  },
  [Operator.YahooFinance]: {
    component: YahooFinanceForm,
  },
  [Operator.Jin10]: {
    component: Jin10Form,
  },
  [Operator.TuShare]: {
    component: TuShareForm,
  },
  [Operator.Crawler]: {
    component: CrawlerForm,
  },
  [Operator.Invoke]: {
    component: InvokeForm,
  },
  [Operator.Concentrator]: {
    component: () => <></>,
  },
  [Operator.Note]: {
    component: () => <></>,
  },
  [Operator.Email]: {
    component: EmailForm,
  },
  [Operator.Iteration]: {
    component: IterationForm,
  },
  [Operator.IterationStart]: {
    component: IterationStartForm,
  },
  [Operator.Tool]: {
    component: ToolForm,
  },
  [Operator.TavilySearch]: {
    component: TavilyForm,
  },
  [Operator.UserFillUp]: {
    component: UserFillUpForm,
  },
  [Operator.StringTransform]: {
    component: StringTransformForm,
  },
  [Operator.TavilyExtract]: {
    component: TavilyExtractForm,
  },
};
