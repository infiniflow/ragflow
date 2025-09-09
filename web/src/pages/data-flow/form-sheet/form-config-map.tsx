import { Operator } from '../constant';
import AgentForm from '../form/agent-form';
import BeginForm from '../form/begin-form';
import CategorizeForm from '../form/categorize-form';
import ChunkerForm from '../form/chunker-form';
import CodeForm from '../form/code-form';
import CrawlerForm from '../form/crawler-form';
import EmailForm from '../form/email-form';
import ExeSQLForm from '../form/exesql-form';
import InvokeForm from '../form/invoke-form';
import IterationForm from '../form/iteration-form';
import IterationStartForm from '../form/iteration-start-from';
import KeywordExtractForm from '../form/keyword-extract-form';
import MessageForm from '../form/message-form';
import ParserForm from '../form/parser-form';
import RelevantForm from '../form/relevant-form';
import RetrievalForm from '../form/retrieval-form/next';
import RewriteQuestionForm from '../form/rewrite-question-form';
import StringTransformForm from '../form/string-transform-form';
import SwitchForm from '../form/switch-form';
import TokenizerForm from '../form/tokenizer-form';
import UserFillUpForm from '../form/user-fill-up-form';

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
  [Operator.KeywordExtract]: {
    component: KeywordExtractForm,
  },
  [Operator.ExeSQL]: {
    component: ExeSQLForm,
  },
  [Operator.Switch]: {
    component: SwitchForm,
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
  [Operator.UserFillUp]: {
    component: UserFillUpForm,
  },
  [Operator.StringTransform]: {
    component: StringTransformForm,
  },
  [Operator.Parser]: {
    component: ParserForm,
  },
  [Operator.Chunker]: {
    component: ChunkerForm,
  },
  [Operator.Tokenizer]: {
    component: TokenizerForm,
  },
};
