export type WikiPageType = 'concept' | 'entity';

/**
 * Parse an internal wiki link href into pageType and slug.
 *
 * Supports the common rendered forms:
 *   artifact/{datasetId}/{pageType}/{slug}
 *   /artifact/{datasetId}/{pageType}/{slug}
 *   {pageType}/{slug}
 *   /{pageType}/{slug}
 *
 * Only entity/ and concept/ links are considered wiki navigation links.
 */
export function parseWikiLinkHref(
  href: string,
): { pageType: WikiPageType; slug: string } | null {
  const normalized = href.trim();
  if (!normalized) return null;

  // Prefer the artifact/{datasetId}/{pageType}/{slug} form.
  const artifactMatch = normalized.match(
    /(?:^|\/)artifact\/[^/]+\/(entity|concept)\/([^/\s"']+)/,
  );
  if (artifactMatch) {
    return {
      pageType: artifactMatch[1] as WikiPageType,
      slug: artifactMatch[2],
    };
  }

  // Fallback to a plain {pageType}/{slug} form.
  const simpleMatch = normalized.match(
    /(?:^|\/)(entity|concept)\/([^/\s"']+)/,
  );
  if (simpleMatch) {
    return {
      pageType: simpleMatch[1] as WikiPageType,
      slug: simpleMatch[2],
    };
  }

  return null;
}

/**
 * Check whether a link href looks like an internal wiki navigation link.
 */
export function isWikiLinkHref(href?: string): boolean {
  return Boolean(href && parseWikiLinkHref(href));
}
