import { Badge } from '@/components/ui/badge';
import { useFetchDatasetArtifactPage } from '@/hooks/use-dataset-artifact-request';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Components } from 'react-markdown';
import ReactMarkdown from 'react-markdown';
import remarkBreaks from 'remark-breaks';
import remarkGfm from 'remark-gfm';
import { ArtifactLinkRenderer } from './artifact-link-renderer';
import { ArtifactNavigatePage } from './artifact-navigate-page';
import { ArtifactPageEditDialog } from './artifact-page-edit-dialog';
import { useArtifactSelection } from './hooks/use-artifact-state';

/**
 * Matches the "## See also" heading the artifact writer emits. We split
 * the markdown there so the body renders as normal prose while the
 * trailing artifactlinks become a styled chip grid.
 */
const SEE_ALSO_RE = /\r?\n##\s+See also\s*\r?\n/i;

/**
 * Matches a See-also entry of the shape
 * ``[label](href)：description`` (or ``:``, the ASCII variant). The
 * description runs to the next newline. Captures label, href, desc.
 */
const SEE_ALSO_ENTRY_WITH_DESC_RE =
  /\[([^\]]+)\]\(([^)]+)\)\s*[:：]\s*([^\n]+)/g;

/**
 * Split `content_md_rendered` into the body markdown and the optional
 * trailing See-also section. The split string is consumed (the heading
 * itself is replaced by our own header in the chip grid).
 */
function splitContentAndSeeAlso(content: string): {
  body: string;
  seeAlso: string | null;
} {
  if (!content) return { body: '', seeAlso: null };
  const match = SEE_ALSO_RE.exec(content);
  if (!match) return { body: content, seeAlso: null };
  return {
    body: content.slice(0, match.index).trimEnd(),
    seeAlso: content.slice(match.index + match[0].length).trim() || null,
  };
}

/**
 * Rewrite See-also entries that carry an inline description after a
 * Chinese-full-width or ASCII colon into native markdown title-syntax:
 *
 *   [entity/象道物流](artifact/.../entity/象道物流)：象屿集团旗下的物流子公司
 *     → [entity/象道物流](artifact/.../entity/象道物流 "象屿集团旗下的物流子公司")
 *
 * react-markdown passes the title attribute through to the rendered
 * ``<a>`` element, so ``ArtifactLinkRenderer`` (which spreads `...rest`)
 * forwards it onto the chip — giving a native HTML tooltip on hover.
 * The description text is consumed by this rewrite so it no longer
 * renders inline next to the chip.
 *
 * Quotes inside the description are backslash-escaped so the title
 * literal stays well-formed.
 */
function attachSeeAlsoTitles(seeAlso: string): string {
  return seeAlso.replace(
    SEE_ALSO_ENTRY_WITH_DESC_RE,
    (_match, label: string, href: string, desc: string) => {
      const cleanDesc = desc.trim().replace(/"/g, '\\"');
      return `[${label}](${href} "${cleanDesc}")`;
    },
  );
}

export function ArtifactViewer() {
  const { t } = useTranslation();
  const { selected, select } = useArtifactSelection();
  const { data, loading } = useFetchDatasetArtifactPage(
    selected?.pageType,
    selected?.slug,
  );
  // Double-click anywhere in the rendered page opens the split-view edit
  // dialog. We don't want a single accidental click on a link or word to
  // open it, so this rides exclusively on the article's onDoubleClick.
  const [editing, setEditing] = useState(false);

  // Stale-URL recovery: the URL holds the slug as the source of truth, so
  // a leftover ``?page_type=…&slug=…`` from a prior session (or a
  // See-also link to a now-deleted page) leaves us with
  // ``selected != null && data == null`` even though the list endpoint
  // is full of valid pages. Drop the URL params so the next render falls
  // into the ``!selected`` branch and the navigate page takes over.
  useEffect(() => {
    if (selected && !loading && data === null) {
      select(null);
    }
  }, [selected, loading, data, select]);

  /**
   * Body markdown overrides:
   * - `p`: first-line indent (2em) and generous vertical rhythm. Keeps the
   *   Chinese-typography convention while remaining sensible for English.
   * - `a`: ArtifactLinkRenderer (intercepts `artifact/...` hrefs and routes
   *   them via URL state).
   */
  const bodyComponents = useMemo<Components>(
    () => ({
      a: ArtifactLinkRenderer,
      p: ({ children, ...rest }) => (
        <p {...rest} className="my-6 leading-7 indent-8 first:mt-0">
          {children}
        </p>
      ),
    }),
    [],
  );

  /**
   * See-also overrides: turn the markdown list into a flex-wrap of pill
   * chips. The `<a>` inside each chip still flows through
   * ArtifactLinkRenderer for navigation. We disable the inherited prose
   * styles on the container via `not-prose` so the chips don't inherit
   * list-marker / margin behaviour.
   */
  const seeAlsoComponents = useMemo<Components>(
    () => ({
      a: ({ children, ...rest }) => (
        <ArtifactLinkRenderer
          {...rest}
          className="inline-flex items-center px-3 py-1.5 rounded-full
            bg-bg-card border border-border-button text-sm
            text-text-primary no-underline
            hover:bg-bg-card-hover hover:border-primary
            transition-colors"
        >
          <span className="truncate max-w-[14rem]">{children}</span>
        </ArtifactLinkRenderer>
      ),
      ul: ({ children }) => (
        <div className="not-prose flex flex-wrap gap-2 mt-3">{children}</div>
      ),
      // Each list item is just a wrapper around the styled <a>; we don't
      // want bullets or extra spacing, so collapse li to a fragment-like
      // span.
      li: ({ children }) => <>{children}</>,
      // The writer's See-also block shouldn't contain prose paragraphs,
      // but guard anyway: render them inline so they don't break the row.
      p: ({ children }) => <>{children}</>,
    }),
    [],
  );

  if (!selected) {
    // First load / no selection — show the Google-style navigate page so
    // the middle pane is never empty on entry.
    return <ArtifactNavigatePage />;
  }

  if (loading && !data) {
    return (
      <div className="flex-1 flex items-center justify-center text-text-secondary">
        {t('common.loading')}
      </div>
    );
  }

  if (!data) {
    // Selection points at a slug that no longer resolves (stale URL,
    // deleted page, bare ``[[slug]]`` See-also link). The effect above
    // is clearing the URL params; in the meantime render the navigate
    // page so the middle pane never lands on a dead-end "no longer
    // exists" message after the list endpoint already returned items.
    return <ArtifactNavigatePage />;
  }

  const { body, seeAlso } = splitContentAndSeeAlso(
    data.content_md_rendered || '',
  );

  return (
    <div className="flex-1 flex flex-col overflow-auto">
      <header className="px-6 pt-6 pb-3 border-b border-border-button">
        <div className="flex items-center gap-2">
          <Badge variant="secondary">{data.page_type}</Badge>
          <span className="text-xs text-text-secondary truncate">
            {data.slug}
          </span>
        </div>
        <h1 className="mt-2 text-2xl font-semibold text-text-primary">
          {data.title}
        </h1>
      </header>
      <article
        className="px-6 py-6 prose max-w-none dark:prose-invert cursor-text"
        onDoubleClick={() => setEditing(true)}
        title={t('artifact.editDialog.doubleClickHint') as string}
      >
        {data.summary ? (
          <section className="not-prose mb-6 p-4 rounded-md bg-bg-card border border-border-button">
            <h2 className="text-xs font-medium uppercase tracking-wide text-text-secondary mb-1">
              {t('artifact.summary')}
            </h2>
            <p className="text-sm text-text-secondary whitespace-pre-wrap leading-relaxed">
              {data.summary}
            </p>
          </section>
        ) : null}

        <ReactMarkdown
          remarkPlugins={[remarkGfm, remarkBreaks]}
          components={bodyComponents}
        >
          {body}
        </ReactMarkdown>

        {seeAlso ? (
          <section className="not-prose mt-10 pt-6 border-t border-border-button">
            <h2 className="flex items-center gap-2 text-xs font-medium uppercase tracking-wide text-text-secondary mb-1">
              {t('artifact.seeAlso')}
            </h2>
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              components={seeAlsoComponents}
            >
              {attachSeeAlsoTitles(seeAlso)}
            </ReactMarkdown>
          </section>
        ) : null}
      </article>

      <ArtifactPageEditDialog
        open={editing}
        pageType={data.page_type ?? null}
        slug={data.slug ?? null}
        onClose={() => setEditing(false)}
      />
    </div>
  );
}
