import { Operator } from '../../constant';
import ArXivForm from './arxiv-form';
import BingForm from './bing-form';
import CrawlerForm from './crawler-form';
import DuckDuckGoForm from './duckduckgo-form';
import EmailForm from './email-form';
import ExeSQLForm from './exesql-form';
import GithubForm from './github-form';
import GoogleForm from './google-form';
import GoogleScholarForm from './google-scholar-form';
import KeenableForm from './keenable-form';
import YouComForm from './youcom-form';
import PubMedForm from './pubmed-form';
import QueritForm from './querit-form';
import BGPTForm from './bgpt-form';
import RetrievalForm from './retrieval-form';
import SearXNGForm from './searxng-form';
import ApiKeyToolForm from './tavily-form';
import WenCaiForm from './wencai-form';
import WikipediaForm from './wikipedia-form';
import YahooFinanceForm from './yahoo-finance-form';

export const ToolFormConfigMap = {
  [Operator.Retrieval]: RetrievalForm,
  [Operator.Code]: () => <div></div>,
  [Operator.DuckDuckGo]: DuckDuckGoForm,
  [Operator.Wikipedia]: WikipediaForm,
  [Operator.PubMed]: PubMedForm,
  [Operator.BGPT]: BGPTForm,
  [Operator.ArXiv]: ArXivForm,
  [Operator.Google]: GoogleForm,
  [Operator.Bing]: BingForm,
  [Operator.GoogleScholar]: GoogleScholarForm,
  [Operator.GitHub]: GithubForm,
  [Operator.ExeSQL]: ExeSQLForm,
  [Operator.YahooFinance]: YahooFinanceForm,
  [Operator.Crawler]: CrawlerForm,
  [Operator.Email]: EmailForm,
  [Operator.TavilySearch]: ApiKeyToolForm,
  [Operator.TavilyExtract]: ApiKeyToolForm,
  [Operator.QueritContents]: ApiKeyToolForm,
  [Operator.QueritSearch]: QueritForm,
  [Operator.WenCai]: WenCaiForm,
  [Operator.SearXNG]: SearXNGForm,
  [Operator.KeenableSearch]: KeenableForm,
  [Operator.YouComSearch]: YouComForm,
};
