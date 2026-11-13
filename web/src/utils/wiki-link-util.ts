export type WikiPageType = 'concept' | 'entity' | 'topic';

/**
 * Parse an internal wiki link href into pageType and slug.
 *
 * Supports the common rendered forms:
 *   artifact/{datasetId}/{pageType}/{slug}
 *   /artifact/{datasetId}/{pageType}/{slug}
 *   {pageType}/{slug}
 *   /{pageType}/{slug}
 *
 * entity/, concept/ and topic/ links are all considered wiki navigation links.
 */
export function parseWikiLinkHref(
  href: string,
): { pageType: WikiPageType; slug: string } | null {
  const normalized = href.trim();
  if (!normalized) return null;

  // Lexical's LinkNode renders scheme-less hrefs by prepending https://, so an
  // internal `artifact/{id}/{pageType}/{slug}` link surfaces in the DOM as
  // `https://artifact/{id}/{pageType}/{slug}` with a bogus "artifact" host.
  // Recognise that normalized form before rejecting URLs with a scheme.
  const lexicalArtifactMatch = normalized.match(
    /^https?:\/\/artifact\/[^/]+\/(entity|concept|topic)\/([^\s"']+)/i,
  );
  if (lexicalArtifactMatch) {
    return {
      pageType: lexicalArtifactMatch[1] as WikiPageType,
      slug: lexicalArtifactMatch[2],
    };
  }

  // Leave URLs with a scheme or authority to normal link navigation.
  if (/^(?:[a-z][a-z\d+.-]*:|[\\/]{2})/i.test(normalized)) return null;

  // Prefer the artifact/{datasetId}/{pageType}/{slug} form.
  const artifactMatch = normalized.match(
    /(?:^|\/)artifact\/[^/]+\/(entity|concept|topic)\/([^\s"']+)/,
  );
  if (artifactMatch) {
    return {
      pageType: artifactMatch[1] as WikiPageType,
      slug: artifactMatch[2],
    };
  }

  // Fallback to a plain {pageType}/{slug} form.
  const simpleMatch = normalized.match(
    /(?:^|\/)(entity|concept|topic)\/([^\s"']+)/,
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
