/**
 * LexicalEditor — WYSIWYG markdown editor using Meta's Lexical framework.
 *
 * Ported from Nimbalyst's NimbalystEditor + Editor.tsx.
 * Markdown import on init, markdown export on change.
 * Wraps content in nimbalyst-editor container for theming.
 */

import { LexicalComposer } from '@lexical/react/LexicalComposer';
import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext';
import { LexicalErrorBoundary } from '@lexical/react/LexicalErrorBoundary';
import { HashtagPlugin } from '@lexical/react/LexicalHashtagPlugin';
import { HistoryPlugin } from '@lexical/react/LexicalHistoryPlugin';
import { LinkPlugin } from '@lexical/react/LexicalLinkPlugin';
import { ListPlugin } from '@lexical/react/LexicalListPlugin';
import { MarkdownShortcutPlugin } from '@lexical/react/LexicalMarkdownShortcutPlugin';
import { RichTextPlugin } from '@lexical/react/LexicalRichTextPlugin';
import { TablePlugin } from '@lexical/react/LexicalTablePlugin';
import { $createParagraphNode, $getRoot } from 'lexical';
import type { JSX } from 'react';
import { useEffect, useMemo, useRef } from 'react';

import ContentEditable from './content-editable';
import theme from './editor-theme';
import {
  $convertFromEnhancedMarkdownString,
  $convertToEnhancedMarkdownString,
  CORE_TRANSFORMERS,
  type Transformer,
} from './markdown';
import nodes from './nodes';

// Direct CSS import (more reliable than @import in App.css)
import './editor-theme.css';

interface Props {
  content: string;
  onChange?: (markdown: string) => void;
  readOnly?: boolean;
  placeholder?: string;
  onToggleSource?: () => void;
  showSource?: boolean;
  onWikiLinkClick?: (pageType: 'concept' | 'entity', slug: string) => void;
}

function InitialContentPlugin({
  content,
  transformers,
}: {
  content: string;
  transformers: Transformer[];
}) {
  const [editor] = useLexicalComposerContext();
  const seeded = useRef(false);

  useEffect(() => {
    // Seed ONCE on mount. Subsequent external content changes are handled by ContentSyncPlugin.
    if (seeded.current) return;
    seeded.current = true;

    editor.update(() => {
      const root = $getRoot();
      root.clear();
      if (content && content.trim()) {
        $convertFromEnhancedMarkdownString(content, transformers);
      } else {
        root.append($createParagraphNode());
      }
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editor]);

  return null;
}

function ContentSyncPlugin({
  content,
  transformers,
}: {
  content: string;
  transformers: Transformer[];
}) {
  const [editor] = useLexicalComposerContext();

  useEffect(() => {
    editor.update(() => {
      const currentMarkdown = $convertToEnhancedMarkdownString(transformers);
      if (currentMarkdown === content) return;

      const root = $getRoot();
      root.clear();
      if (content && content.trim()) {
        $convertFromEnhancedMarkdownString(content, transformers);
      } else {
        root.append($createParagraphNode());
      }
    });
  }, [content, editor, transformers]);

  return null;
}

function ChangeListener({
  onChange,
  transformers,
}: {
  onChange?: (md: string) => void;
  transformers: Transformer[];
}) {
  const [editor] = useLexicalComposerContext();
  const initRef = useRef(false);

  useEffect(() => {
    if (!onChange) return;
    const removeListener = editor.registerUpdateListener(
      ({ dirtyElements, dirtyLeaves }) => {
        if (!initRef.current) {
          initRef.current = true;
          return;
        }
        if (dirtyElements.size === 0 && dirtyLeaves.size === 0) return;
        const markdown = editor.read(() =>
          $convertToEnhancedMarkdownString(transformers),
        );
        onChange(markdown);
      },
    );
    return removeListener;
  }, [editor, onChange, transformers]);

  return null;
}

import FloatingSelectionToolbar from './plugins/floating-selection-toolbar';
import TableActionsPlugin from './plugins/table-actions-plugin';
import ToolbarPlugin from './plugins/toolbar-plugin';
import { WikiLinkClickPlugin } from './plugins/wiki-link-click-plugin';

export default function LexicalEditor({
  content,
  onChange,
  readOnly = false,
  placeholder = 'Start writing...',
  onToggleSource,
  showSource = false,
  onWikiLinkClick,
}: Props): JSX.Element {
  const transformers = useMemo(() => CORE_TRANSFORMERS, []);

  const initialConfig = useMemo(
    () => ({
      namespace: 'LlmWikiEditor',
      nodes: [...nodes],
      theme,
      editable: !readOnly,
      onError: (error: Error) => {
        console.error('[LexicalEditor] Error:', error);
      },
    }),
    [readOnly],
  );

  return (
    <div className="nimbalyst-editor">
      <div className="editor-shell">
        <LexicalComposer initialConfig={initialConfig}>
          <div className="editor-container">
            <ToolbarPlugin
              onToggleSource={onToggleSource || (() => {})}
              showSource={showSource}
            />
            <InitialContentPlugin
              content={content}
              transformers={transformers}
            />
            <ContentSyncPlugin content={content} transformers={transformers} />
            <ChangeListener onChange={onChange} transformers={transformers} />
            <FloatingSelectionToolbar />
            <RichTextPlugin
              contentEditable={
                <div className="nim-editor-scroller">
                  <div className="nim-editor">
                    <ContentEditable placeholder={placeholder} />
                  </div>
                </div>
              }
              ErrorBoundary={LexicalErrorBoundary}
            />
            <HistoryPlugin />
            <MarkdownShortcutPlugin transformers={transformers} />
            <ListPlugin />
            <LinkPlugin />
            <WikiLinkClickPlugin onWikiLinkClick={onWikiLinkClick} />
            <HashtagPlugin />
            <TablePlugin />
            <TableActionsPlugin />
          </div>
        </LexicalComposer>
      </div>
    </div>
  );
}
