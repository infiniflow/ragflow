import { CodeHighlightNode, CodeNode } from '@lexical/code';
import { AutoFocusPlugin } from '@lexical/react/LexicalAutoFocusPlugin';
import {
  InitialConfigType,
  LexicalComposer,
} from '@lexical/react/LexicalComposer';
import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext';
import { ContentEditable } from '@lexical/react/LexicalContentEditable';
import { LexicalErrorBoundary } from '@lexical/react/LexicalErrorBoundary';
import { HistoryPlugin } from '@lexical/react/LexicalHistoryPlugin';
import { RichTextPlugin } from '@lexical/react/LexicalRichTextPlugin';
import { HeadingNode, QuoteNode } from '@lexical/rich-text';
import { EditorState, Klass, LexicalNode } from 'lexical';
import { useEffect, useState } from 'react';

import theme from './theme';
import { VariableNode } from './variable-node';
import VariablePickerMenuPlugin from './variable-picker-plugin';

// Catch any errors that occur during Lexical updates and log them
// or throw them as needed. If you don't throw them, Lexical will
// try to recover gracefully without losing user data.
function onError(error: Error) {
  console.error(error);
}

type MyOnChangePluginProps = { onChange: (editorState: EditorState) => void };

const Nodes: Array<Klass<LexicalNode>> = [
  HeadingNode,
  QuoteNode,
  CodeHighlightNode,
  CodeNode,
  VariableNode,
];

function MyOnChangePlugin({ onChange }: MyOnChangePluginProps) {
  const [editor] = useLexicalComposerContext();
  useEffect(() => {
    return editor.registerUpdateListener(({ editorState }) => {
      onChange(editorState);
    });
  }, [editor, onChange]);
  return null;
}

export function PromptEditor() {
  const initialConfig: InitialConfigType = {
    namespace: 'MyEditor',
    theme,
    onError,
    nodes: Nodes,
    // html: { import: buildImportMap() },

    // editorState() {
    //   const root = $getRoot();
    //   if (root.getFirstChild() === null) {
    //     const quote = $createQuoteNode();
    //     quote.append(
    //       $createTextNode(
    //         `In case you were wondering what the black box at the bottom is – it's the debug view, showing the current state of the editor. ` +
    //           `You can disable it by pressing on the settings control in the bottom-left of your screen and toggling the debug view setting.`,
    //       ),
    //     );
    //     root.append(quote);
    //   }
    // },
  };

  const [editorState, setEditorState] = useState<EditorState>();
  function onChange(editorState: EditorState) {
    const editorStateJSON = editorState.toJSON();
    console.log('🚀 ~ onChange ~ editorStateJSON:', editorStateJSON);
    setEditorState(editorState);
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
      <MyOnChangePlugin onChange={onChange} />
      <VariablePickerMenuPlugin></VariablePickerMenuPlugin>
    </LexicalComposer>
  );
}
