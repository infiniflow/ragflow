/**
 * FloatingSelectionToolbar — appears when text is selected,
 * provides formatting: heading, bold, italic, code, strikethrough, link, quote.
 */
import { $createCodeNode, $isCodeNode } from '@lexical/code';
import { TOGGLE_LINK_COMMAND } from '@lexical/link';
import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext';
import type { HeadingTagType } from '@lexical/rich-text';
import {
  $createHeadingNode,
  $createQuoteNode,
  $isHeadingNode,
  $isQuoteNode,
} from '@lexical/rich-text';
import { $setBlocksType } from '@lexical/selection';
import {
  $createParagraphNode,
  $getRoot,
  $getSelection,
  $isElementNode,
  $isRangeSelection,
  COMMAND_PRIORITY_LOW,
  FORMAT_TEXT_COMMAND,
  SELECTION_CHANGE_COMMAND,
} from 'lexical';
import { useEffect, useRef, useState } from 'react';

export default function FloatingSelectionToolbar() {
  const [editor] = useLexicalComposerContext();
  const [show, setShow] = useState(false);
  const [pos, setPos] = useState({ x: 0, y: 0 });
  const barRef = useRef<HTMLDivElement>(null);
  const [currentBlock, setCurrentBlock] = useState<string>('paragraph');

  useEffect(() => {
    const removeUpdateListener = editor.registerUpdateListener(() => {
      editor.getEditorState().read(() => {
        const selection = $getSelection();
        if (!$isRangeSelection(selection) || selection.isCollapsed()) {
          setShow(false);
          return;
        }
        const nativeSel = window.getSelection();
        if (!nativeSel || nativeSel.isCollapsed || !nativeSel.rangeCount) {
          setShow(false);
          return;
        }

        // Detect current block type
        const anchor = selection.anchor.getNode();
        let block = 'paragraph';
        let parent = $isElementNode(anchor) ? anchor : anchor.getParent();
        while (parent && parent !== $getRoot()) {
          if ($isHeadingNode(parent)) {
            block = parent.getTag();
            break;
          }
          if ($isQuoteNode(parent)) {
            block = 'quote';
            break;
          }
          if ($isCodeNode(parent)) {
            block = 'code';
            break;
          }
          parent = parent.getParent();
        }
        setCurrentBlock(block);

        const range = nativeSel.getRangeAt(0);
        const rect = range.getBoundingClientRect();
        setPos({
          x: rect.left + rect.width / 2,
          y: rect.top - 10,
        });
        setShow(true);
      });
    });

    const removeSelectionListener = editor.registerCommand(
      SELECTION_CHANGE_COMMAND,
      () => false,
      COMMAND_PRIORITY_LOW,
    );

    return () => {
      removeUpdateListener();
      removeSelectionListener();
    };
  }, [editor]);

  const format = (type: 'bold' | 'italic' | 'strikethrough' | 'code') => {
    editor.dispatchCommand(FORMAT_TEXT_COMMAND, type);
    setShow(false);
  };

  const toggleLink = () => {
    const url = window.prompt('Enter URL:');
    if (url) {
      editor.dispatchCommand(TOGGLE_LINK_COMMAND, url);
    }
    setShow(false);
  };

  const setHeading = (tag: HeadingTagType) => {
    editor.update(() => {
      const selection = $getSelection();
      if ($isRangeSelection(selection)) {
        $setBlocksType(selection, () => $createHeadingNode(tag));
      }
    });
    setShow(false);
  };

  const setQuote = () => {
    editor.update(() => {
      const selection = $getSelection();
      if ($isRangeSelection(selection)) {
        $setBlocksType(selection, () => $createQuoteNode());
      }
    });
    setShow(false);
  };

  const setParagraph = () => {
    editor.update(() => {
      const selection = $getSelection();
      if ($isRangeSelection(selection)) {
        $setBlocksType(selection, () => $createParagraphNode());
      }
    });
    setShow(false);
  };

  const [showCodeLang, setShowCodeLang] = useState(false);

  const setCodeBlock = (lang?: string) => {
    editor.update(() => {
      const selection = $getSelection();
      if ($isRangeSelection(selection)) {
        const node = $createCodeNode(lang);
        $setBlocksType(selection, () => node);
      }
    });
    setShow(false);
  };

  const codeLangs = [
    '',
    'javascript',
    'typescript',
    'python',
    'html',
    'css',
    'scss',
    'less',
    'json',
    'yaml',
    'xml',
    'bash',
    'sh',
    'powershell',
    'sql',
    'graphql',
    'go',
    'rust',
    'cpp',
    'c',
    'csharp',
    'java',
    'kotlin',
    'swift',
    'php',
    'ruby',
    'perl',
    'lua',
    'r',
    'matlab',
    'dart',
    'zig',
    'makefile',
    'dockerfile',
    'ini',
    'toml',
    'diff',
    'markdown',
    'mermaid',
    'plaintext',
  ];

  const [showHeadingMenu, setShowHeadingMenu] = useState(false);

  if (!show) return null;

  const btnStyle: React.CSSProperties = {
    padding: '4px 7px',
    border: 'none',
    borderRadius: 4,
    cursor: 'pointer',
    fontSize: 12,
    fontWeight: 500,
    background: 'transparent',
    color: 'var(--nim-text)',
    fontFamily: 'inherit',
    lineHeight: 1,
    whiteSpace: 'nowrap',
  };
  const activeStyle: React.CSSProperties = {
    ...btnStyle,
    background: 'var(--nim-bg-selected)',
    color: 'var(--nim-primary)',
  };

  const headingLabels: Record<string, string> = {
    h1: 'H1',
    h2: 'H2',
    h3: 'H3',
    h4: 'H4',
    h5: 'H5',
    h6: 'H6',
  };

  return (
    <div
      ref={barRef}
      style={{
        position: 'fixed',
        left: pos.x,
        top: pos.y,
        transform: 'translate(-50%, -100%)',
        zIndex: 1000,
        display: 'flex',
        alignItems: 'center',
        gap: 2,
        padding: '4px 6px',
        background: 'var(--bg-base)',
        border: '1px solid var(--nim-border)',
        borderRadius: 8,
        boxShadow: '0 4px 12px rgba(0,0,0,0.25)',
      }}
    >
      {/* Heading dropdown (H1-H6 only) */}
      <div style={{ position: 'relative' }}>
        <button
          style={{
            ...btnStyle,
            padding: '4px 14px 4px 7px',
            border: '1px solid var(--nim-border)',
            borderRadius: 4,
            background: 'var(--nim-bg-tertiary)',
            fontSize: 11,
            fontWeight: 600,
          }}
          onClick={() => setShowHeadingMenu(!showHeadingMenu)}
          title="Heading"
        >
          {headingLabels[currentBlock] || 'H'} ▾
        </button>
        {showHeadingMenu && (
          <div
            style={{
              position: 'absolute',
              top: '100%',
              left: 0,
              marginTop: 4,
              background: 'var(--nim-bg)',
              border: '1px solid var(--nim-border)',
              borderRadius: 6,
              boxShadow: '0 4px 12px rgba(0,0,0,0.25)',
              minWidth: 100,
              zIndex: 1001,
              padding: '4px 0',
            }}
          >
            {(['h1', 'h2', 'h3', 'h4', 'h5', 'h6'] as const).map((tag) => (
              <div
                key={tag}
                onClick={() => {
                  setShowHeadingMenu(false);
                  setHeading(tag);
                }}
                style={{
                  padding: '5px 12px',
                  cursor: 'pointer',
                  fontSize: 12,
                  fontWeight: tag === 'h1' ? 700 : tag === 'h2' ? 600 : 500,
                  color:
                    currentBlock === tag
                      ? 'var(--nim-primary)'
                      : 'var(--nim-text)',
                  background:
                    currentBlock === tag
                      ? 'var(--nim-bg-selected)'
                      : 'transparent',
                }}
                onMouseEnter={(e) => {
                  e.currentTarget.style.background = 'var(--nim-bg-hover)';
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.background =
                    currentBlock === tag
                      ? 'var(--nim-bg-selected)'
                      : 'transparent';
                }}
              >
                {headingLabels[tag]}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Block type buttons: Paragraph, Quote */}
      <button
        style={currentBlock === 'paragraph' ? activeStyle : btnStyle}
        onClick={setParagraph}
        title="Paragraph"
      >
        ¶
      </button>
      <button
        style={currentBlock === 'quote' ? activeStyle : btnStyle}
        onClick={setQuote}
        title="Quote"
      >
        ❝
      </button>

      {/* Code Block with language picker */}
      <div style={{ position: 'relative' }}>
        <button
          style={currentBlock === 'code' ? activeStyle : btnStyle}
          onClick={() => setShowCodeLang(!showCodeLang)}
          title="Code Block"
        >
          {'{}'}
        </button>
        {showCodeLang && (
          <div
            style={{
              position: 'absolute',
              top: '100%',
              left: '50%',
              transform: 'translateX(-50%)',
              marginTop: 4,
              background: 'var(--nim-bg)',
              border: '1px solid var(--nim-border)',
              borderRadius: 6,
              boxShadow: '0 4px 12px rgba(0,0,0,0.25)',
              minWidth: 130,
              maxHeight: 200,
              overflowY: 'auto',
              zIndex: 1001,
              padding: '4px 0',
            }}
          >
            {codeLangs.map((lang) => (
              <div
                key={lang || '__plain'}
                role="button"
                tabIndex={-1}
                onClick={() => {
                  setShowCodeLang(false);
                  setCodeBlock(lang || undefined);
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    setShowCodeLang(false);
                    setCodeBlock(lang || undefined);
                  }
                }}
                style={{
                  padding: '4px 12px',
                  cursor: 'pointer',
                  fontSize: 11,
                  color: 'var(--nim-text)',
                }}
                onMouseEnter={(e) => {
                  e.currentTarget.style.background = 'var(--nim-bg-hover)';
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.background = 'transparent';
                }}
              >
                {lang || '(no language)'}
              </div>
            ))}
          </div>
        )}
      </div>

      <div
        style={{
          width: 1,
          height: 16,
          background: 'var(--nim-border)',
          margin: '0 3px',
        }}
      />

      {/* Inline format controls */}
      <button style={btnStyle} onClick={() => format('bold')} title="Bold">
        <strong>B</strong>
      </button>
      <button
        style={{ ...btnStyle, fontStyle: 'italic' }}
        onClick={() => format('italic')}
        title="Italic"
      >
        <em>I</em>
      </button>
      <button
        style={{ ...btnStyle, textDecoration: 'line-through' }}
        onClick={() => format('strikethrough')}
        title="Strikethrough"
      >
        <s>S</s>
      </button>
      <button
        style={{ ...btnStyle, fontFamily: 'monospace' }}
        onClick={() => format('code')}
        title="Inline Code"
      >
        &lt;/&gt;
      </button>
      <div
        style={{
          width: 1,
          height: 16,
          background: 'var(--nim-border)',
          margin: '0 3px',
        }}
      />

      {/* Link */}
      <button style={btnStyle} onClick={toggleLink} title="Link">
        🔗
      </button>
    </div>
  );
}
