import { CodeHighlightNode, CodeNode } from '@lexical/code';
import {
  InitialConfigType,
  LexicalComposer,
} from '@lexical/react/LexicalComposer';
import { ContentEditable } from '@lexical/react/LexicalContentEditable';
import { LexicalErrorBoundary } from '@lexical/react/LexicalErrorBoundary';
import { RichTextPlugin } from '@lexical/react/LexicalRichTextPlugin';
import { HeadingNode, QuoteNode } from '@lexical/rich-text';
import {
  $getRoot,
  $nodesOfType,
  EditorState,
  Klass,
  LexicalNode,
} from 'lexical';

import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import theme from './theme';
import { VariableNode } from './variable-node';
import { VariableOnChangePlugin } from './variable-on-change-plugin';
import VariablePickerMenuPlugin from './variable-picker-plugin';

// Catch any errors that occur during Lexical updates and log them
// or throw them as needed. If you don't throw them, Lexical will
// try to recover gracefully without losing user data.
function onError(error: Error) {
  console.error(error);
}

const Nodes: Array<Klass<LexicalNode>> = [
  HeadingNode,
  QuoteNode,
  CodeHighlightNode,
  CodeNode,
  VariableNode,
];

type IProps = {
  value?: string;
  onChange?: (value?: string) => void;
};

export function PromptEditor({ value, onChange }: IProps) {
  const { t } = useTranslation();
  const initialConfig: InitialConfigType = {
    namespace: 'PromptEditor',
    theme,
    onError,
    nodes: Nodes,
  };

  const onValueChange = useCallback(
    (editorState: EditorState) => {
      editorState?.read(() => {
        const listNodes = $nodesOfType(VariableNode); // to be removed
        // const allNodes = $dfs();
        console.log('🚀 ~ onChange ~ allNodes:', listNodes);

        const text = $getRoot().getTextContent();

        onChange?.(text);
      });
    },
    [onChange],
  );

  return (
    <LexicalComposer initialConfig={initialConfig}>
      <RichTextPlugin
        contentEditable={
          <ContentEditable className="min-h-40 relative px-2 py-1 border" />
        }
        placeholder={
          <div className="absolute top-2 left-2">{t('common.pleaseInput')}</div>
        }
        ErrorBoundary={LexicalErrorBoundary}
      />
      <VariablePickerMenuPlugin value={value}></VariablePickerMenuPlugin>
      <VariableOnChangePlugin onChange={onValueChange}></VariableOnChangePlugin>
    </LexicalComposer>
  );
}
