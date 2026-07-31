/**
 * RawMarkdownEditor — raw markdown editing using Monaco Editor.
 * Theme follows the app's Nimbalyst theme system.
 */

import type { OnChange, OnMount } from '@monaco-editor/react';
import Editor from '@monaco-editor/react';
import { Eye } from 'lucide-react';
import type { editor as monacoEditor } from 'monaco-editor';
import type { JSX } from 'react';
import { useEffect, useRef } from 'react';

import { useTheme } from '@/components/theme-provider';

interface Props {
  content: string;
  onChange?: (value: string) => void;
  readOnly?: boolean;
  onToggleSource?: () => void;
  language?: string;
}

function defineTheme(monaco: typeof import('monaco-editor'), isDark: boolean) {
  const name = isDark ? 'llmwiki-dark' : 'llmwiki-light';
  monaco.editor.defineTheme(name, {
    base: isDark ? 'vs-dark' : 'vs',
    inherit: true,
    rules: [],
    colors: isDark
      ? {
          'editor.background': '#2d2d2d',
          'editor.foreground': '#ffffff',
          'editor.lineHighlightBackground': '#3a3a3a',
          'editor.selectionBackground': 'rgba(96,165,250,0.2)',
          'editorCursor.foreground': '#60a5fa',
          'editorLineNumber.foreground': '#808080',
          'editorLineNumber.activeForeground': '#b3b3b3',
          'editor.selectionHighlightBackground': 'rgba(96,165,250,0.1)',
        }
      : {
          'editor.background': '#ffffff',
          'editor.foreground': '#111827',
          'editor.lineHighlightBackground': '#f3f4f6',
          'editor.selectionBackground': 'rgba(59,130,246,0.2)',
          'editorCursor.foreground': '#3b82f6',
          'editorLineNumber.foreground': '#9ca3af',
          'editorLineNumber.activeForeground': '#6b7280',
          'editor.selectionHighlightBackground': 'rgba(59,130,246,0.1)',
        },
  });
  return name;
}

export default function RawMarkdownEditor({
  content,
  onChange,
  readOnly = false,
  onToggleSource,
  language = 'markdown',
}: Props & { showSource?: boolean }): JSX.Element {
  const { theme } = useTheme();
  const editorRef = useRef<monacoEditor.IStandaloneCodeEditor | null>(null);
  const monacoRef = useRef<typeof import('monaco-editor') | null>(null);
  const themeApplied = useRef<string>('');

  const handleMount: OnMount = (editor, monaco) => {
    editorRef.current = editor;
    monacoRef.current = monaco;
    const isDark = theme === 'dark';
    const name = defineTheme(monaco, isDark);
    monaco.editor.setTheme(name);
    themeApplied.current = name;
  };

  // Switch Monaco theme when app theme changes
  useEffect(() => {
    if (!monacoRef.current) return;
    const isDark = theme === 'dark';
    const expected = isDark ? 'llmwiki-dark' : 'llmwiki-light';
    if (themeApplied.current === expected) return;
    const name = defineTheme(monacoRef.current, isDark);
    monacoRef.current.editor.setTheme(name);
    themeApplied.current = name;
  }, [theme]);

  const handleChange: OnChange = (value) => {
    if (onChange && value !== undefined) {
      onChange(value);
    }
  };

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        flex: 1,
        minHeight: 0,
      }}
    >
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
          className="nim-mode-btn inline-flex items-center justify-center gap-1 px-2.5 py-1 text-xs font-medium border border-accent-primary rounded bg-accent-primary-5 text-accent-primary hover:bg-accent-primary-10 transition-colors"
          title="WYSIWYG"
        >
          <Eye className="size-3.5" />
          WYSIWYG
        </button>
      </div>
      <div style={{ flex: 1, minHeight: 0 }}>
        <Editor
          height="100%"
          defaultLanguage={language}
          value={content}
          onChange={handleChange}
          onMount={handleMount}
          options={{
            readOnly,
            minimap: { enabled: false },
            fontSize: 14,
            fontFamily: "'SF Mono', 'JetBrains Mono', 'Fira Code', monospace",
            lineNumbers: 'on',
            scrollBeyondLastLine: false,
            wordWrap: 'on',
            tabSize: 2,
            renderLineHighlight: 'line',
            cursorBlinking: 'smooth',
            smoothScrolling: true,
            padding: { top: 16 },
            automaticLayout: true,
          }}
        />
      </div>
    </div>
  );
}
