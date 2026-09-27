/**
 * ToolbarPlugin — editor toolbar with Toggle Source and Table of Contents.
 */
import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext';
import { $isHeadingNode, type HeadingTagType } from '@lexical/rich-text';
import {
  $getNodeByKey,
  $getRoot,
  $isElementNode,
  type LexicalNode,
} from 'lexical';
import { Code } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';

interface TOCItem {
  text: string;
  tag: HeadingTagType;
  key: string;
}

interface Props {
  onToggleSource: () => void;
  showSource: boolean;
}

export default function ToolbarPlugin({ onToggleSource, showSource }: Props) {
  const [editor] = useLexicalComposerContext();
  const [showToc, setShowToc] = useState(false);
  const [tocItems, setTocItems] = useState<TOCItem[]>([]);

  // Scan headings for TOC
  const scanHeadings = useCallback(() => {
    editor.read(() => {
      const items: TOCItem[] = [];
      const visit = (node: LexicalNode) => {
        if ($isHeadingNode(node)) {
          items.push({
            text: node.getTextContent(),
            tag: node.getTag(),
            key: node.__key,
          });
        }
        if ($isElementNode(node)) {
          for (const child of node.getChildren()) {
            visit(child);
          }
        }
      };
      visit($getRoot());
      setTocItems(items);
    });
  }, [editor]);

  useEffect(() => {
    if (showToc) scanHeadings();
  }, [showToc, scanHeadings]);

  const navigateToHeading = (key: string) => {
    editor.update(() => {
      const node = $getNodeByKey(key);
      if (node && 'selectStart' in node) {
        (node as any).selectStart();
        (node as any).scrollIntoView?.();
      }
    });
    setShowToc(false);
  };

  const tagToPadding: Record<string, string> = {
    h1: '0',
    h2: '12px',
    h3: '24px',
    h4: '36px',
    h5: '48px',
    h6: '60px',
  };

  return (
    <>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 6,
          padding: '4px 12px',
          borderBottom: '1px solid var(--nim-border)',
          background: 'var(--nim-toolbar-bg)',
          flexShrink: 0,
        }}
      >
        <button
          type="button"
          onClick={onToggleSource}
          className="nim-mode-btn inline-flex items-center justify-center gap-1 px-2 py-1 text-xs font-medium rounded border transition-colors"
          style={{
            borderColor: showSource ? 'var(--nim-primary)' : 'transparent',
            background: showSource ? 'var(--nim-bg-selected)' : 'transparent',
            color: showSource ? 'var(--nim-primary)' : 'var(--nim-text-muted)',
          }}
          title="Source"
        >
          <Code className="size-3.5" />
          Source
        </button>
        <button
          type="button"
          onClick={() => {
            setShowToc(!showToc);
          }}
          className="nim-mode-btn"
          style={{
            padding: '4px 10px',
            border: showToc
              ? '1px solid var(--nim-primary)'
              : '1px solid transparent',
            borderRadius: 4,
            cursor: 'pointer',
            fontSize: 12,
            fontWeight: 500,
            background: showToc ? 'var(--nim-bg-selected)' : 'transparent',
            color: showToc ? 'var(--nim-primary)' : 'var(--nim-text-muted)',
            fontFamily: 'inherit',
          }}
        >
          ☰ Outline
        </button>
      </div>

      {/* TOC Dropdown */}
      {showToc && tocItems.length > 0 && (
        <div
          style={{
            position: 'absolute',
            top: 40,
            left: 80,
            zIndex: 50,
            background: 'var(--nim-bg)',
            border: '1px solid var(--nim-border)',
            borderRadius: 8,
            boxShadow: '0 4px 16px rgba(0,0,0,0.3)',
            maxHeight: 300,
            overflowY: 'auto',
            minWidth: 200,
          }}
        >
          <div
            style={{
              padding: '8px 12px',
              fontSize: 11,
              fontWeight: 600,
              color: 'var(--nim-text-faint)',
              textTransform: 'uppercase',
              letterSpacing: 0.5,
              borderBottom: '1px solid var(--nim-border)',
            }}
          >
            Table of Contents
          </div>
          {tocItems.map((item, i) => (
            <div
              key={item.key}
              onClick={() => navigateToHeading(item.key)}
              style={{
                padding: `6px 12px 6px calc(12px + ${tagToPadding[item.tag] || '0px'})`,
                fontSize: 13,
                cursor: 'pointer',
                color: 'var(--nim-text)',
                borderLeft: `3px solid ${item.tag === 'h1' ? 'var(--nim-primary)' : 'transparent'}`,
              }}
              onMouseEnter={(e) =>
                (e.currentTarget.style.background = 'var(--nim-bg-hover)')
              }
              onMouseLeave={(e) =>
                (e.currentTarget.style.background = 'transparent')
              }
            >
              {item.text || `(heading ${i + 1})`}
            </div>
          ))}
        </div>
      )}
    </>
  );
}
