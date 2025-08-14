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
  $getSelection,
  $nodesOfType,
  EditorState,
  Klass,
  LexicalNode,
} from 'lexical';

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext';
import { Variable } from 'lucide-react';
import { ReactNode, useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { PasteHandlerPlugin } from './paste-handler-plugin';
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

type PromptContentProps = { showToolbar?: boolean; multiLine?: boolean };

type IProps = {
  value?: string;
  onChange?: (value?: string) => void;
  placeholder?: ReactNode;
} & PromptContentProps;

function PromptContent({
  showToolbar = true,
  multiLine = true,
}: PromptContentProps) {
  const [editor] = useLexicalComposerContext();
  const [isBlur, setIsBlur] = useState(false);
  const { t } = useTranslation();

  const insertTextAtCursor = useCallback(() => {
    editor.update(() => {
      const selection = $getSelection();

      if (selection !== null) {
        selection.insertText(' /');
      }
    });
  }, [editor]);

  const handleVariableIconClick = useCallback(() => {
    insertTextAtCursor();
  }, [insertTextAtCursor]);

  const handleBlur = useCallback(() => {
    setIsBlur(true);
  }, []);

  const handleFocus = useCallback(() => {
    setIsBlur(false);
  }, []);

  return (
    <section
      className={cn('border rounded-sm ', { 'border-blue-400': !isBlur })}
    >
      {showToolbar && (
        <div className="border-b px-2 py-2 justify-end flex">
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="inline-block cursor-pointer cursor p-0.5 hover:bg-gray-100 dark:hover:bg-slate-800 rounded-sm">
                <Variable size={16} onClick={handleVariableIconClick} />
              </span>
            </TooltipTrigger>
            <TooltipContent>
              <p>{t('flow.insertVariableTip')}</p>
            </TooltipContent>
          </Tooltip>
        </div>
      )}
      <ContentEditable
        className={cn(
          'relative px-2 py-1 focus-visible:outline-none max-h-[50vh] overflow-auto',
          {
            'min-h-40': multiLine,
          },
        )}
        onBlur={handleBlur}
        onFocus={handleFocus}
      />
    </section>
  );
}

export function PromptEditor({
  value,
  onChange,
  placeholder,
  showToolbar,
  multiLine = true,
}: IProps) {
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
    <div className="relative">
      <LexicalComposer initialConfig={initialConfig}>
        <RichTextPlugin
          contentEditable={
            <PromptContent
              showToolbar={showToolbar}
              multiLine={multiLine}
            ></PromptContent>
          }
          placeholder={
            <div
              className={cn(
                'absolute top-1 left-2 text-text-secondary pointer-events-none',
                {
                  'truncate w-[90%]': !multiLine,
                  'translate-y-10': multiLine,
                },
              )}
            >
              {placeholder || t('common.promptPlaceholder')}
            </div>
          }
          ErrorBoundary={LexicalErrorBoundary}
        />
        <VariablePickerMenuPlugin value={value}></VariablePickerMenuPlugin>
        <PasteHandlerPlugin />
        <VariableOnChangePlugin
          onChange={onValueChange}
        ></VariableOnChangePlugin>
      </LexicalComposer>
    </div>
  );
}
