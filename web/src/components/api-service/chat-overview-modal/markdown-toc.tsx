import React, { useEffect, useState } from 'react';
import Anchor, { AnchorItem } from './anchor';

interface MarkdownTocProps {
  content: string;
}

const MarkdownToc: React.FC<MarkdownTocProps> = ({ content }) => {
  const [items, setItems] = useState<AnchorItem[]>([]);

  useEffect(() => {
    const generateTocItems = () => {
      const headings = document.querySelectorAll(
        '.wmde-markdown h2, .wmde-markdown h3',
      );

      // If headings haven't rendered yet, wait for next frame
      if (headings.length === 0) {
        requestAnimationFrame(generateTocItems);
        return;
      }

      const tocItems: AnchorItem[] = [];
      let currentH2Item: AnchorItem | null = null;

      headings.forEach((heading) => {
        const title = heading.textContent || '';
        const id = heading.id;
        const isH2 = heading.tagName.toLowerCase() === 'h2';

        if (id && title) {
          const item: AnchorItem = {
            key: id,
            href: `#${id}`,
            title,
          };

          if (isH2) {
            currentH2Item = item;
            tocItems.push(item);
          } else {
            if (currentH2Item) {
              if (!currentH2Item.children) {
                currentH2Item.children = [];
              }
              currentH2Item.children.push(item);
            } else {
              tocItems.push(item);
            }
          }
        }
      });

      setItems(tocItems.slice(1));
    };

    // Use requestAnimationFrame to ensure execution after DOM rendering
    requestAnimationFrame(() => {
      requestAnimationFrame(generateTocItems);
    });
  }, [content]);

  return (
    <div
      className="markdown-toc bg-bg-base text-text-primary shadow shadow-text-secondary"
      style={{
        position: 'fixed',
        right: 30,
        top: 100,
        bottom: 150,
        width: 200,
        padding: '10px',
        maxHeight: 'calc(100vh - 170px)',
        overflowY: 'auto',
        zIndex: 100,
      }}
    >
      <Anchor items={items} />
    </div>
  );
};

export default MarkdownToc;
