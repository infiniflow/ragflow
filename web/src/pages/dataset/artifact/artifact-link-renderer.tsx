import { Routes } from '@/routes';
import { ComponentProps } from 'react';
import { useNavigate, useParams } from 'react-router';

/**
 * Match ``artifact/<kb_id>/<rest>`` — the form emitted by the writer's
 * ``_artifact_transform_links``. ``<rest>`` is either ``<page_type>/<slug>``
 * (the well-formed case the LLM should produce) or just ``<slug>`` (which
 * happens when the LLM dropped the type prefix). Both flow through the
 * same in-place navigation; the bare-slug case lands on a missing page
 * which the viewer renders as "This page no longer exists".
 */
const ARTIFACT_LINK_RE = /^artifact\/([^/]+)\/(.+)$/;

function parseArtifactRest(rest: string): { pageType: string; slug: string } {
  const slashIdx = rest.indexOf('/');
  if (slashIdx < 0) return { pageType: '', slug: rest };
  return {
    pageType: rest.slice(0, slashIdx),
    slug: rest.slice(slashIdx + 1),
  };
}

function buildArtifactSearch(pageType: string, slug: string): string {
  const slugQ = `slug=${encodeURIComponent(slug)}`;
  return pageType
    ? `?page_type=${encodeURIComponent(pageType)}&${slugQ}`
    : `?${slugQ}`;
}

/**
 * Custom <a> renderer for react-markdown that intercepts markdown links of
 * the form ``artifact/<kb_id>/<page_type>/<slug>`` (the form the artifact
 * writer emits via ``content_md_rendered``).
 *
 * All artifact links navigate in-place using react-router:
 *  - Same KB → push the new selection into the query string
 *    (`?page_type=&slug=`). The list and the viewer both react to the
 *    change via `useArtifactSelection`.
 *  - Different KB → navigate to that KB's artifact route with the same
 *    query string, still in the current tab.
 * Anything that isn't an artifact link falls through to a plain external
 * link (opens in a new tab).
 */
export function ArtifactLinkRenderer({
  href,
  children,
  ...rest
}: ComponentProps<'a'>) {
  const navigate = useNavigate();
  const params = useParams<{ id?: string }>();

  if (typeof href !== 'string') {
    return (
      <a {...rest} href={href} target="_blank" rel="noreferrer">
        {children}
      </a>
    );
  }

  const m = ARTIFACT_LINK_RE.exec(href);
  if (!m) {
    // Not an artifact link — treat as external.
    return (
      <a {...rest} href={href} target="_blank" rel="noreferrer">
        {children}
      </a>
    );
  }

  const [, linkKbId, restPath] = m;
  const { pageType, slug } = parseArtifactRest(restPath);
  const search = buildArtifactSearch(pageType, slug);

  const isSameKb = !params.id || params.id === linkKbId;
  const internalHref = isSameKb
    ? search
    : `/dataset${Routes.Artifact}/${linkKbId}${search}`;

  return (
    <a
      {...rest}
      href={internalHref}
      onClick={(e) => {
        // Honor modifier-clicks (ctrl/cmd/middle-click) so users can still
        // open in a new tab on purpose. Default click navigates in-place.
        if (
          e.defaultPrevented ||
          e.button !== 0 ||
          e.metaKey ||
          e.ctrlKey ||
          e.shiftKey ||
          e.altKey
        ) {
          return;
        }
        e.preventDefault();
        if (isSameKb) {
          navigate({ search });
        } else {
          navigate(internalHref);
        }
      }}
    >
      {children}
    </a>
  );
}
