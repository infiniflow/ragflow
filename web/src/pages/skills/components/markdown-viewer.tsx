import React, { memo } from 'react';
import ReactMarkdown from 'react-markdown';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { oneLight } from 'react-syntax-highlighter/dist/esm/styles/prism';
import remarkGfm from 'remark-gfm';

interface MarkdownViewerProps {
  content: string;
}

// Remove YAML frontmatter from content
const removeFrontmatter = (content: string): string => {
  const lines = content.split('\n');
  if (lines[0]?.trim() === '---') {
    const endIndex = lines.slice(1).findIndex((line) => line.trim() === '---');
    if (endIndex !== -1) {
      return lines.slice(endIndex + 2).join('\n');
    }
  }
  return content;
};

const MarkdownViewer: React.FC<MarkdownViewerProps> = ({ content }) => {
  const cleanContent = removeFrontmatter(content);

  return (
    <div className="markdown-body" style={{ maxWidth: 900, margin: '0 auto' }}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          h1: ({ children }) => (
            <h1
              style={{
                fontSize: '2em',
                fontWeight: 'bold',
                marginBottom: '0.5em',
              }}
            >
              {children}
            </h1>
          ),
          h2: ({ children }) => (
            <h2
              style={{
                fontSize: '1.5em',
                fontWeight: 'bold',
                marginTop: '1em',
                marginBottom: '0.5em',
              }}
            >
              {children}
            </h2>
          ),
          h3: ({ children }) => (
            <h3
              style={{
                fontSize: '1.25em',
                fontWeight: 'bold',
                marginTop: '1em',
                marginBottom: '0.5em',
              }}
            >
              {children}
            </h3>
          ),
          h4: ({ children }) => (
            <h4
              style={{
                fontSize: '1.1em',
                fontWeight: 'bold',
                marginTop: '1em',
                marginBottom: '0.5em',
              }}
            >
              {children}
            </h4>
          ),
          code: ({ className, children }) => {
            const match = /language-(\w+)/.exec(className || '');
            const language = match ? match[1] : '';

            if (language) {
              return (
                <SyntaxHighlighter
                  style={oneLight}
                  language={language}
                  PreTag="div"
                >
                  {String(children).replace(/\n$/, '')}
                </SyntaxHighlighter>
              );
            }

            return <code className={className}>{children}</code>;
          },
          img: ({ src, alt }) => (
            <img
              src={src}
              alt={alt}
              style={{ maxWidth: '100%', height: 'auto', borderRadius: 4 }}
            />
          ),
          table: ({ children }) => (
            <table
              style={{
                width: '100%',
                borderCollapse: 'collapse',
                marginBottom: 16,
              }}
            >
              {children}
            </table>
          ),
          th: ({ children }) => (
            <th
              style={{
                border: '1px solid #d9d9d9',
                padding: '8px 12px',
                backgroundColor: '#fafafa',
                fontWeight: 600,
              }}
            >
              {children}
            </th>
          ),
          td: ({ children }) => (
            <td
              style={{
                border: '1px solid #d9d9d9',
                padding: '8px 12px',
              }}
            >
              {children}
            </td>
          ),
        }}
      >
        {cleanContent}
      </ReactMarkdown>
    </div>
  );
};

export default memo(MarkdownViewer);
