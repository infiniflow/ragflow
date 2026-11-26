import MarkdownPreview, {
  type MarkdownPreviewProps,
} from '@uiw/react-markdown-preview';
import { Loader2 } from 'lucide-react';
import { memo, startTransition, useEffect, useMemo, useState } from 'react';

interface MarkdownPart {
  source: string;
  prefixLength: number;
  offset: number;
}

function splitMarkdown(source: string): MarkdownPart[] {
  const parts: MarkdownPart[] = [];
  let content = '';
  let headings = '';
  let prefix = '';
  let fence = '';
  let offset = 0;

  const flush = () => {
    if (!content) return;
    parts.push({
      source: prefix + content,
      prefixLength: prefix.length,
      offset,
    });
    offset += content.length;
    prefix = headings;
    content = '';
  };

  for (const line of source.match(/[^\n]*\n|[^\n]+$/g) ?? []) {
    const marker = /^ {0,3}(`{3,}|~{3,})(.*)/.exec(line);
    if (marker) {
      if (!fence) {
        fence = marker[1];
      } else if (
        marker[1][0] === fence[0] &&
        marker[1].length >= fence.length &&
        !marker[2].trim()
      ) {
        fence = '';
      }
    }
    const heading = !fence && /^ {0,3}(#{1,6})(?:[ \t]+|$)/.exec(line);
    if (heading) {
      if (heading[1].length <= 3) flush();
      headings += `${line.trimEnd()}\n\n`;
    }
    content += line;
  }
  flush();
  return parts;
}

const MarkdownSection = memo(function MarkdownSection({
  part,
}: {
  part: MarkdownPart;
}) {
  // Earlier headings preserve the renderer's document-wide slug numbering,
  // but only this section's actual content is included in the DOM.
  const removeHeadingContext: MarkdownPreviewProps['rehypeRewrite'] = (
    node,
  ) => {
    if (node.type !== 'root') return;
    const first = node.children.findIndex(
      (child) => (child.position?.start.offset ?? -1) >= part.prefixLength,
    );
    node.children = first < 0 ? [] : node.children.slice(first);
  };

  return (
    <MarkdownPreview
      className="mb-6 last:mb-0"
      source={part.source}
      rehypeRewrite={removeHeadingContext}
    />
  );
});

const ProgressiveMarkdown = memo(function ProgressiveMarkdown({
  source,
  isDarkTheme,
}: {
  source: string;
  isDarkTheme: boolean;
}) {
  const parts = useMemo(() => splitMarkdown(source), [source]);
  const [visibleCount, setVisibleCount] = useState(0);

  useEffect(() => {
    if (visibleCount >= parts.length) return;
    // Yield between sections so input can run before parsing the next endpoint.
    const timer = window.setTimeout(
      () => startTransition(() => setVisibleCount((count) => count + 1)),
      0,
    );
    return () => window.clearTimeout(timer);
  }, [parts.length, visibleCount]);

  return (
    <div data-color-mode={isDarkTheme ? 'dark' : 'light'}>
      {parts.slice(0, visibleCount).map((part) => (
        <MarkdownSection key={part.offset} part={part} />
      ))}
      {visibleCount < parts.length && (
        <div className="flex justify-center py-10">
          <Loader2 className="size-5 animate-spin text-text-secondary" />
        </div>
      )}
    </div>
  );
});

export default ProgressiveMarkdown;
