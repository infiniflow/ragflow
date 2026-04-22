import { useIsDarkTheme } from '@/components/theme-provider';
import { Tag, Typography, theme } from 'antd';
import React, { memo } from 'react';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import {
  oneDark,
  oneLight,
} from 'react-syntax-highlighter/dist/esm/styles/prism';

const { Text } = Typography;
const { useToken } = theme;

interface CodeViewerProps {
  content: string;
  filename: string;
}

const EXT_LANG: Record<string, string> = {
  ts: 'typescript',
  tsx: 'tsx',
  js: 'javascript',
  jsx: 'jsx',
  py: 'python',
  rs: 'rust',
  go: 'go',
  rb: 'ruby',
  java: 'java',
  kt: 'kotlin',
  swift: 'swift',
  c: 'c',
  cpp: 'cpp',
  h: 'c',
  hpp: 'cpp',
  cs: 'csharp',
  css: 'css',
  scss: 'scss',
  less: 'less',
  html: 'html',
  xml: 'xml',
  json: 'json',
  yaml: 'yaml',
  yml: 'yaml',
  toml: 'toml',
  sh: 'bash',
  bash: 'bash',
  zsh: 'bash',
  sql: 'sql',
  dockerfile: 'docker',
  lua: 'lua',
  r: 'r',
  dart: 'dart',
  php: 'php',
  pl: 'perl',
  ex: 'elixir',
  exs: 'elixir',
  erl: 'erlang',
  hs: 'haskell',
  vim: 'vim',
  ini: 'ini',
  cfg: 'ini',
};

const getLang = (filename: string): string => {
  const lower = filename.toLowerCase();
  if (lower === 'dockerfile' || lower.startsWith('dockerfile.'))
    return 'docker';
  if (lower === 'makefile' || lower === 'gnumakefile') return 'makefile';
  const ext = lower.split('.').pop() ?? '';
  return EXT_LANG[ext] || ext || 'text';
};

const CodeViewer: React.FC<CodeViewerProps> = ({ content, filename }) => {
  const isDarkTheme = useIsDarkTheme();
  const { token } = useToken();
  const language = getLang(filename);

  const lineCount = content.split('\n').length;
  const charCount = content.length;

  // Format file size
  const formatSize = (bytes: number): string => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  return (
    <div>
      {/* File Header */}
      <div
        style={{
          padding: '12px 16px',
          backgroundColor: token.colorBgElevated,
          borderBottom: `1px solid ${token.colorBorderSecondary}`,
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
        }}
      >
        <Text strong>{filename}</Text>
        <div>
          <Tag color="blue">{language}</Tag>
          <Text type="secondary" style={{ fontSize: 12, marginLeft: 8 }}>
            {lineCount} lines | {formatSize(charCount)}
          </Text>
        </div>
      </div>

      {/* Code Content */}
      <div className="bg-bg-component">
        <SyntaxHighlighter
          language={language}
          style={isDarkTheme ? oneDark : oneLight}
          showLineNumbers
          lineNumberStyle={{ minWidth: 40, paddingRight: 16 }}
          customStyle={{
            margin: 0,
            padding: '16px',
            fontSize: 13,
            lineHeight: 1.6,
            backgroundColor: 'transparent',
          }}
        >
          {content || '// Empty file'}
        </SyntaxHighlighter>
      </div>
    </div>
  );
};

export default memo(CodeViewer);
