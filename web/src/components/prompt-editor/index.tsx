import { CodeHighlightNode, CodeNode } from '@lexical/code';
import { AutoFocusPlugin } from '@lexical/react/LexicalAutoFocusPlugin';
import {
  InitialConfigType,
  LexicalComposer,
} from '@lexical/react/LexicalComposer';
import { ContentEditable } from '@lexical/react/LexicalContentEditable';
import { LexicalErrorBoundary } from '@lexical/react/LexicalErrorBoundary';
import { HistoryPlugin } from '@lexical/react/LexicalHistoryPlugin';
import { OnChangePlugin } from '@lexical/react/LexicalOnChangePlugin';
import { RichTextPlugin } from '@lexical/react/LexicalRichTextPlugin';
import { HeadingNode, QuoteNode } from '@lexical/rich-text';
import {
  $getRoot,
  $nodesOfType,
  EditorState,
  Klass,
  LexicalNode,
} from 'lexical';

import theme from './theme';
import { VariableNode } from './variable-node';
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
  const initialConfig: InitialConfigType = {
    namespace: 'MyEditor',
    theme,
    onError,
    nodes: Nodes,
  };

  function onValueChange(editorState: EditorState) {
    editorState?.read(() => {
      const listNodes = $nodesOfType(VariableNode);
      // const allNodes = $dfs();
      console.log('🚀 ~ onChange ~ allNodes:', listNodes);

      const text = $getRoot().getTextContent();
      console.log('🚀 ~ editorState?.read ~ x:', text);
      onChange?.(text);
    });
  }

  return (
    <LexicalComposer initialConfig={initialConfig}>
      <RichTextPlugin
        contentEditable={
          <ContentEditable className="min-h-40 relative px-2 py-1 border" />
        }
        placeholder={
          <div className="absolute top-2 left-2">Enter some text...</div>
        }
        ErrorBoundary={LexicalErrorBoundary}
      />
      <HistoryPlugin />
      <AutoFocusPlugin />
      {/* <MyOnChangePlugin onChange={onChange} /> */}
      <VariablePickerMenuPlugin value={value}></VariablePickerMenuPlugin>
      <OnChangePlugin onChange={onValueChange}></OnChangePlugin>
    </LexicalComposer>
  );
}
