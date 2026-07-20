import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext';
import {
  parseWikiLinkHref,
  type WikiPageType,
} from '@/pages/dataset/compilation/utils/parse-wiki-link';
import { useEffect } from 'react';

type WikiLinkClickPluginProps = {
  onWikiLinkClick?: (pageType: WikiPageType, slug: string) => void;
};

function findAnchorElement(
  target: EventTarget | null,
): HTMLAnchorElement | null {
  if (!(target instanceof HTMLElement)) return null;
  return target.closest('a[href]') as HTMLAnchorElement | null;
}

export function WikiLinkClickPlugin({
  onWikiLinkClick,
}: WikiLinkClickPluginProps) {
  const [editor] = useLexicalComposerContext();

  useEffect(() => {
    if (!onWikiLinkClick) return;

    const rootElement = editor.getRootElement();
    if (!rootElement) return;

    const handleClick = (event: MouseEvent) => {
      const anchor = findAnchorElement(event.target);
      if (!anchor) return;

      const href = anchor.getAttribute('href');
      if (!href) return;

      const parsed = parseWikiLinkHref(href);
      if (!parsed) return;

      event.preventDefault();
      event.stopPropagation();
      onWikiLinkClick(parsed.pageType, parsed.slug);
    };

    rootElement.addEventListener('click', handleClick);
    return () => rootElement.removeEventListener('click', handleClick);
  }, [editor, onWikiLinkClick]);

  return null;
}
