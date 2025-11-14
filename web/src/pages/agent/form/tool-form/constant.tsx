import { Operator } from '../../constant';
import AkShareForm from '../akshare-form';
import ArXivForm from './arxiv-form';
import BingForm from './bing-form';
import CrawlerForm from './crawler-form';
import DuckDuckGoForm from './duckduckgo-form';
import EmailForm from './email-form';
import ExeSQLForm from './exesql-form';
import GithubForm from './github-form';
import GoogleForm from './google-form';
import GoogleScholarForm from './google-scholar-form';
import PubMedForm from './pubmed-form';
import RetrievalForm from './retrieval-form';
import SearXNGForm from './searxng-form';
import TavilyForm from './tavily-form';
import WenCaiForm from './wencai-form';
import WikipediaForm from './wikipedia-form';
import YahooFinanceForm from './yahoo-finance-form';

export const ToolFormConfigMap = {
  [Operator.Retrieval]: RetrievalForm,
  [Operator.Code]: () => <div></div>,
  [Operator.DuckDuckGo]: DuckDuckGoForm,
  [Operator.Wikipedia]: WikipediaForm,
  [Operator.PubMed]: PubMedForm,
  [Operator.ArXiv]: ArXivForm,
  [Operator.Google]: GoogleForm,
  [Operator.Bing]: BingForm,
  [Operator.GoogleScholar]: GoogleScholarForm,
  [Operator.GitHub]: GithubForm,
  [Operator.ExeSQL]: ExeSQLForm,
  [Operator.AkShare]: AkShareForm,
  [Operator.YahooFinance]: YahooFinanceForm,
  [Operator.Crawler]: CrawlerForm,
  [Operator.Email]: EmailForm,
  [Operator.TavilySearch]: TavilyForm,
  [Operator.TavilyExtract]: TavilyForm,
  [Operator.WenCai]: WenCaiForm,
  [Operator.SearXNG]: SearXNGForm,
};
