import React, { useEffect, useState } from 'react';
import Anchor, { AnchorItem } from './anchor';

interface MarkdownTocProps {
  // A ref to the container that wraps the markdown preview this TOC tracks.
  // Headings are queried only inside this element so the TOC can never pick up
  // headings rendered by another markdown instance.
  container: React.RefObject<HTMLElement>;
}

const MarkdownToc: React.FC<MarkdownTocProps> = ({ container }) => {
  const [items, setItems] = useState<AnchorItem[]>([]);

  useEffect(() => {
    const root = container.current;
    if (!root) return;

    let active = true;

    const buildToc = () => {
      const headings = root.querySelectorAll(
        '.wmde-markdown h2, .wmde-markdown h3',
      );
      if (headings.length === 0) return false;

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

      if (active) setItems(tocItems.slice(1));
      return true;
    };

    // Build immediately if the preview already rendered its headings.
    if (buildToc()) return;

    // Otherwise wait for the lazy preview to inject its DOM, then build once.
    // This replaces the previous per-frame requestAnimationFrame loop, which
    // polled forever when the lazy chunk failed to load and leaked on unmount.
    const observer = new MutationObserver(() => {
      if (buildToc()) observer.disconnect();
    });
    observer.observe(root, { childList: true, subtree: true });

    return () => {
      active = false;
      observer.disconnect();
    };
  }, [container]);

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
