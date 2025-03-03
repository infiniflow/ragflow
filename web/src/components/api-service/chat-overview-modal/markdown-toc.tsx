import { Anchor } from 'antd';
import type { AnchorLinkItemProps } from 'antd/es/anchor/Anchor';
import React, { useEffect, useState } from 'react';

interface MarkdownTocProps {
  content: string;
}

const MarkdownToc: React.FC<MarkdownTocProps> = ({ content }) => {
  const [items, setItems] = useState<AnchorLinkItemProps[]>([]);

  useEffect(() => {
    const generateTocItems = () => {
      const headings = document.querySelectorAll(
        '.wmde-markdown h2, .wmde-markdown h3',
      );
      const tocItems: AnchorLinkItemProps[] = [];
      let currentH2Item: AnchorLinkItemProps | null = null;

      headings.forEach((heading) => {
        const title = heading.textContent || '';
        const id = heading.id;
        const isH2 = heading.tagName.toLowerCase() === 'h2';

        if (id && title) {
          const item: AnchorLinkItemProps = {
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

    setTimeout(generateTocItems, 100);
  }, [content]);

  return (
    <div
      className="markdown-toc"
      style={{
        position: 'fixed',
        right: 20,
        top: 100,
        width: 200,
        background: '#fff',
        padding: '10px',
        maxHeight: 'calc(100vh - 170px)',
        overflowY: 'auto',
        zIndex: 1000,
      }}
    >
      <Anchor items={items} affix={false} />
    </div>
  );
};

export default MarkdownToc;
