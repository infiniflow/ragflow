import { useIsDarkTheme } from '@/components/theme-provider';
import React, { memo } from 'react';
import ReactMarkdown from 'react-markdown';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import {
  oneDark,
  oneLight,
} from 'react-syntax-highlighter/dist/esm/styles/prism';
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
  const isDarkTheme = useIsDarkTheme();
  const cleanContent = removeFrontmatter(content);

  return (
    <div className="markdown-body max-w-[900px] mx-auto">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          h1: ({ children }) => (
            <h1 className="text-3xl font-bold mb-2 text-text-primary">
              {children}
            </h1>
          ),
          h2: ({ children }) => (
            <h2 className="text-2xl font-bold mt-4 mb-2 text-text-primary">
              {children}
            </h2>
          ),
          h3: ({ children }) => (
            <h3 className="text-xl font-bold mt-4 mb-2 text-text-primary">
              {children}
            </h3>
          ),
          h4: ({ children }) => (
            <h4 className="text-lg font-bold mt-4 mb-2 text-text-primary">
              {children}
            </h4>
          ),
          p: ({ children }) => (
            <p className="text-text-primary mb-2 leading-relaxed">{children}</p>
          ),
          code: ({ className, children }) => {
            const match = /language-(\w+)/.exec(className || '');
            const language = match ? match[1] : '';

            if (language) {
              return (
                <SyntaxHighlighter
                  style={isDarkTheme ? oneDark : oneLight}
                  language={language}
                  PreTag="div"
                  customStyle={{
                    backgroundColor: 'var(--bg-component)',
                    borderRadius: '8px',
                    marginBottom: '1em',
                  }}
                >
                  {String(children).replace(/\n$/, '')}
                </SyntaxHighlighter>
              );
            }

            return (
              <code
                className={`${className} bg-bg-elevated text-text-primary px-1.5 py-0.5 rounded font-mono text-sm`}
              >
                {children}
              </code>
            );
          },
          img: ({ src, alt }) => (
            <img src={src} alt={alt} className="max-w-full h-auto rounded" />
          ),
          table: ({ children }) => (
            <table className="w-full border-collapse mb-4">{children}</table>
          ),
          th: ({ children }) => (
            <th className="border border-border-secondary px-3 py-2 bg-bg-elevated font-semibold text-text-primary text-left">
              {children}
            </th>
          ),
          td: ({ children }) => (
            <td className="border border-border-secondary px-3 py-2 text-text-primary">
              {children}
            </td>
          ),
          li: ({ children }) => (
            <li className="text-text-primary">{children}</li>
          ),
          a: ({ children, href }) => (
            <a href={href} className="text-accent-primary hover:underline">
              {children}
            </a>
          ),
          blockquote: ({ children }) => (
            <blockquote className="border-l-4 border-border-secondary pl-4 italic text-text-secondary my-4">
              {children}
            </blockquote>
          ),
          hr: () => <hr className="border-border-secondary my-4" />,
          pre: ({ children }) => (
            <pre className="bg-bg-elevated rounded-lg p-4 overflow-x-auto mb-4">
              {children}
            </pre>
          ),
          ul: ({ children }) => (
            <ul className="list-disc list-inside mb-4 text-text-primary">
              {children}
            </ul>
          ),
          ol: ({ children }) => (
            <ol className="list-decimal list-inside mb-4 text-text-primary">
              {children}
            </ol>
          ),
          strong: ({ children }) => (
            <strong className="font-bold text-text-primary">{children}</strong>
          ),
          em: ({ children }) => (
            <em className="italic text-text-primary">{children}</em>
          ),
        }}
      >
        {cleanContent}
      </ReactMarkdown>
    </div>
  );
};

export default memo(MarkdownViewer);
