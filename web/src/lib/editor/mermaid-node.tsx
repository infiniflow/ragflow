/**
 * MermaidNode - A Lexical DecoratorNode for rendering Mermaid diagrams.
 * Ported from Nimbalyst — supports click-to-edit source code.
 */

/* eslint-disable @typescript-eslint/no-unused-vars */

import { addClassNamesToElement } from '@lexical/utils';
import type {
  DOMConversionMap,
  DOMConversionOutput,
  DOMExportOutput,
  EditorConfig,
  LexicalEditor,
  LexicalNode,
  NodeKey,
  SerializedLexicalNode,
  Spread,
} from 'lexical';
import { $applyNodeReplacement, DecoratorNode } from 'lexical';
import type { JSX } from 'react';

export interface MermaidPayload {
  content: string;
  key?: NodeKey;
}

export type SerializedMermaidNode = Spread<
  { content: string },
  SerializedLexicalNode
>;

export class MermaidNode extends DecoratorNode<JSX.Element> {
  __content: string;

  constructor(content: string, key?: NodeKey) {
    super(key);
    this.__content = content;
  }

  static getType(): string {
    return 'mermaid';
  }

  static clone(node: MermaidNode): MermaidNode {
    return new MermaidNode(node.__content, node.__key);
  }

  static importJSON(serializedNode: SerializedMermaidNode): MermaidNode {
    return $createMermaidNode({ content: serializedNode.content });
  }

  exportJSON(): SerializedMermaidNode {
    return {
      content: this.__content,
      type: 'mermaid',
      version: 1,
    };
  }

  createDOM(_config: EditorConfig, _editor: LexicalEditor): HTMLElement {
    const div = document.createElement('div');
    addClassNamesToElement(div, 'mermaid-container');
    return div;
  }

  updateDOM(_prevNode: MermaidNode, _dom: HTMLElement): boolean {
    return _prevNode.__content !== this.__content;
  }

  exportDOM(_editor: LexicalEditor): DOMExportOutput {
    const element = document.createElement('div');
    element.classList.add('mermaid-container');
    const pre = document.createElement('pre');
    const code = document.createElement('code');
    code.classList.add('language-mermaid');
    code.textContent = this.__content;
    pre.appendChild(code);
    element.appendChild(pre);
    return { element };
  }

  static importDOM(): DOMConversionMap | null {
    return {
      div: (domNode: HTMLElement) => {
        if (!domNode.classList.contains('mermaid-container')) return null;
        return { conversion: convertMermaidElement, priority: 1 };
      },
    };
  }

  getContent(): string {
    return this.__content;
  }

  getTextContent(): string {
    return this.__content;
  }

  setContent(content: string): void {
    const writable = this.getWritable();
    writable.__content = content;
  }

  decorate(_editor: LexicalEditor, _config: EditorConfig): JSX.Element {
    return (
      <MermaidComponent
        content={this.__content}
        nodeKey={this.__key}
        editor={_editor}
      />
    );
  }
}

function convertMermaidElement(
  domNode: HTMLElement,
): DOMConversionOutput | null {
  const codeElement = domNode.querySelector('code.language-mermaid');
  if (codeElement) {
    const node = $createMermaidNode({ content: codeElement.textContent || '' });
    return { node };
  }
  return null;
}

export function $createMermaidNode(payload?: MermaidPayload): MermaidNode {
  const content = payload?.content || 'graph TD\n  A[Start] --> B[End]';
  return $applyNodeReplacement(new MermaidNode(content, payload?.key));
}

export function $isMermaidNode(
  node: LexicalNode | null | undefined,
): node is MermaidNode {
  return node instanceof MermaidNode;
}

// ─── React rendering component ─────────────────────────────

import { useTheme } from '@/components/theme-provider';
import { $getNodeByKey } from 'lexical';
import mermaid from 'mermaid';
import { useCallback, useEffect, useRef, useState } from 'react';

let mermaidInitialized = false;
function ensureMermaidInit(themeName: string) {
  const mermaidTheme = themeName === 'dark' ? 'dark' : 'default';
  if (!mermaidInitialized) {
    mermaid.initialize({
      startOnLoad: false,
      theme: mermaidTheme,
      securityLevel: 'antiscript',
      fontFamily: 'monospace',
    });
    mermaidInitialized = true;
  } else {
    mermaidInitialized = false;
    mermaid.initialize({
      startOnLoad: false,
      theme: mermaidTheme,
      securityLevel: 'antiscript',
      fontFamily: 'monospace',
    });
    mermaidInitialized = true;
  }
}

// Sequential render queue — avoids freezing with many diagrams
let renderQueue = Promise.resolve();
function queueRender(fn: () => Promise<void>): void {
  renderQueue = renderQueue.then(
    () =>
      new Promise<void>((resolve) => {
        setTimeout(async () => {
          await fn();
          resolve();
        }, 30);
      }),
  );
}

interface MermaidComponentProps {
  content: string;
  nodeKey: NodeKey;
  editor: LexicalEditor;
}

function MermaidComponent({ content, nodeKey, editor }: MermaidComponentProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const [state, setState] = useState<'loading' | 'error' | 'done'>('loading');
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  const [editContent, setEditContent] = useState(content);
  const { theme } = useTheme();

  // ── Render/re-render diagram ──
  const doRender = useCallback(
    async (source: string) => {
      if (!containerRef.current) return;
      try {
        const id = `mermaid_${nodeKey}_${Date.now()}`;
        const { svg } = await mermaid.render(id, source);
        if (containerRef.current) {
          containerRef.current.innerHTML = svg;
          setState('done');
          setError(null);
        }
      } catch (err: any) {
        setState('error');
        setError(err?.message || String(err));
      }
    },
    [nodeKey],
  );

  useEffect(() => {
    let mounted = true;
    ensureMermaidInit(theme);

    queueRender(async () => {
      if (!mounted) return;
      if (!containerRef.current) return;
      await doRender(content);
    });

    return () => {
      mounted = false;
    };
  }, [content, nodeKey, theme, doRender, editing]);

  // ── Click to edit ──
  const handleDiagramClick = () => {
    setEditContent(content);
    setEditing(true);
  };

  // ── Commit changes back to Lexical node ──
  const commitChanges = useCallback(() => {
    const newContent = editContent.trim();
    if (newContent && newContent !== content) {
      editor.update(() => {
        const node = $getNodeByKey(nodeKey);
        if ($isMermaidNode(node)) {
          node.setContent(newContent);
        }
      });
    }
    setEditing(false);
  }, [editContent, content, editor, nodeKey]);

  // ── Live preview while editing (debounced) ──
  const debounceRef = useRef<ReturnType<typeof setTimeout>>();
  const handleEditChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const val = e.target.value;
    setEditContent(val);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(async () => {
      if (val.trim()) {
        ensureMermaidInit(theme);
        await doRender(val);
      }
    }, 350);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
      commitChanges();
    }
    if (e.key === 'Escape') {
      setEditing(false);
      setEditContent(content);
    }
  };

  // Error state
  if (state === 'error' && !editing) {
    return (
      <div
        className="mermaid-block"
        style={{
          margin: '12px 0',
          padding: 16,
          border: '1px solid var(--nim-error)',
          borderRadius: 8,
          background: 'color-mix(in srgb, var(--nim-error) 8%, transparent)',
          cursor: 'pointer',
        }}
        onClick={handleDiagramClick}
      >
        <pre
          style={{
            margin: 0,
            fontSize: 12,
            color: 'var(--nim-error)',
            whiteSpace: 'pre-wrap',
          }}
        >
          {content}
        </pre>
        <div
          style={{ fontSize: 11, color: 'var(--nim-text-faint)', marginTop: 8 }}
        >
          {error} — click to edit source
        </div>
      </div>
    );
  }

  // Edit mode
  if (editing) {
    return (
      <div className="mermaid-block" style={{ margin: '12px 0' }}>
        <div
          ref={containerRef}
          className="mermaid-render-container"
          style={{
            overflowX: 'auto',
            minHeight: 60,
            marginBottom: 8,
            border: '1px solid var(--nim-border)',
            borderRadius: 6,
            padding: 8,
            background: 'var(--nim-bg)',
          }}
        />
        <textarea
          ref={textareaRef}
          value={editContent}
          onChange={handleEditChange}
          onKeyDown={handleKeyDown}
          onBlur={commitChanges}
          autoFocus
          style={{
            width: '100%',
            minHeight: 80,
            padding: '8px 10px',
            fontSize: 13,
            fontFamily: 'Menlo, Consolas, monospace',
            lineHeight: 1.5,
            border: '1px solid var(--nim-border-focus)',
            borderRadius: 6,
            background: 'var(--nim-code-bg)',
            color: 'var(--nim-code-text)',
            resize: 'vertical',
            outline: 'none',
            boxSizing: 'border-box',
          }}
          placeholder="Edit mermaid source..."
        />
        <div
          style={{
            fontSize: 11,
            color: 'var(--nim-text-faint)',
            marginTop: 4,
            display: 'flex',
            gap: 12,
          }}
        >
          <span>Ctrl+Enter to apply</span>
          <span>Esc to cancel</span>
        </div>
      </div>
    );
  }

  // Normal display mode with border + edit button above
  return (
    <div
      className="mermaid-block"
      style={{
        margin: '16px 0',
        border: '1px solid var(--nim-border)',
        borderRadius: 8,
        overflow: 'hidden',
      }}
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '4px 10px',
          background: 'var(--nim-bg-tertiary)',
          borderBottom: '1px solid var(--nim-border)',
          fontSize: 12,
        }}
      >
        <span style={{ color: 'var(--nim-text-muted)', fontWeight: 600 }}>
          mermaid
        </span>
        <button
          onClick={(e) => {
            e.stopPropagation();
            setEditContent(content);
            setEditing(true);
          }}
          style={{
            padding: '2px 8px',
            border: '1px solid var(--nim-border)',
            borderRadius: 4,
            background: 'var(--nim-bg)',
            color: 'var(--nim-text-muted)',
            cursor: 'pointer',
            fontSize: 11,
            fontFamily: 'inherit',
          }}
        >
          Edit
        </button>
      </div>
      <div
        style={{ textAlign: 'center', cursor: 'pointer', padding: 8 }}
        onClick={handleDiagramClick}
        title="Click to edit source"
      >
        <div
          ref={containerRef}
          className="mermaid-render-container"
          style={{
            overflowX: 'auto',
            minHeight: state === 'loading' ? 60 : 'auto',
          }}
        />
      </div>
    </div>
  );
}
