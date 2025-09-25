import { Operator } from '../constant';
import ContextForm from '../form/context-form';
import HierarchicalMergerForm from '../form/hierarchical-merger-form';
import ParserForm from '../form/parser-form';
import SplitterForm from '../form/splitter-form';
import TokenizerForm from '../form/tokenizer-form';

export const FormConfigMap = {
  [Operator.Begin]: {
    component: () => <></>,
  },
  [Operator.Note]: {
    component: () => <></>,
  },
  [Operator.Parser]: {
    component: ParserForm,
  },
  [Operator.Tokenizer]: {
    component: TokenizerForm,
  },
  [Operator.Splitter]: {
    component: SplitterForm,
  },
  [Operator.HierarchicalMerger]: {
    component: HierarchicalMergerForm,
  },
  [Operator.Context]: {
    component: ContextForm,
  },
};
