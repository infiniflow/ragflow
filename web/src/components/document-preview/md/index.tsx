import message from '@/components/ui/message';
import { Spin } from '@/components/ui/spin';
import { MarkdownRemarkPluginsLite } from '@/constants/markdown-remark-plugins';
import { cn } from '@/lib/utils';
import FileError from '@/pages/document-viewer/file-error';
import request from '@/utils/request';
import React, { useEffect, useState } from 'react';
import ReactMarkdown from 'react-markdown';

interface MdProps {
  // filePath: string;
  className?: string;
  url: string;
}

export const Md: React.FC<MdProps> = ({ url, className }) => {
  const [content, setContent] = useState<string>('');
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!url) {
      setContent('');
      setError(null);
      setLoading(false);
      return;
    }

    let cancelled = false;
    setError(null);
    setLoading(true);

    const fetchMarkdown = async () => {
      try {
        const res = await request(url, {
          method: 'GET',
          responseType: 'blob',
          onError: (err: unknown) => {
            console.error('Error loading markdown file:', err);
          },
        });
        if (cancelled) return;

        const blob = res.data;
        const text = await blob.text();
        if (cancelled) return;
        setContent(text);
      } catch (err: unknown) {
        if (cancelled) return;
        const messageText =
          err instanceof Error ? err.message : 'Failed to fetch markdown file';
        setError(messageText);
        message.error('Failed to load file');
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };

    fetchMarkdown();
    return () => {
      cancelled = true;
    };
  }, [url]);

  if (error) return <FileError>{error}</FileError>;

  return (
    <div
      style={{ padding: 4, overflow: 'scroll' }}
      className={cn(
        className,
        'markdown-body relative h-[calc(100vh - 200px)]',
      )}
    >
      {loading && (
        <div className="absolute inset-0 flex items-center justify-center">
          <Spin />
        </div>
      )}
      {!loading && (
        <article className="prose max-w-none dark:prose-invert prose-headings:text-text-primary prose-p:text-text-primary prose-strong:text-text-primary prose-li:text-text-primary prose-blockquote:text-text-secondary prose-blockquote:border-border-secondary prose-a:text-accent-primary prose-a:no-underline hover:prose-a:underline prose-code:text-text-primary prose-code:bg-bg-elevated prose-code:rounded prose-code:px-1.5 prose-code:py-0.5 prose-code:before:content-[''] prose-code:after:content-[''] prose-pre:bg-bg-elevated prose-th:bg-bg-elevated prose-th:text-text-primary prose-td:text-text-primary prose-hr:border-border-secondary">
          <ReactMarkdown remarkPlugins={MarkdownRemarkPluginsLite}>
            {content}
          </ReactMarkdown>
        </article>
      )}
    </div>
  );
};

export default Md;
