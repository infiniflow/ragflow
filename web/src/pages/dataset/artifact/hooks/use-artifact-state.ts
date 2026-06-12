import { useCallback, useMemo } from 'react';
import { useSearchParams } from 'react-router';

export interface SelectedArtifact {
  pageType: string;
  /** Slug tail (the portion after the page type — what backend expects). */
  slug: string;
}

export type ArtifactView = 'page' | 'graph';

/**
 * URL-driven selection state for the Artifact tab. The query string holds
 * `?page_type=…&slug=…` which makes every selection bookmarkable and lets
 * browser back/forward navigate page history.
 *
 * Both the left list (`<ArtifactList>`) and the right viewer
 * (`<ArtifactViewer>`) consume the same source of truth via this hook —
 * which also avoids the "two parents tracking the same selection" bug.
 */
export function useArtifactSelection() {
  const [searchParams, setSearchParams] = useSearchParams();

  const selected = useMemo<SelectedArtifact | null>(() => {
    const slug = searchParams.get('slug');
    if (!slug) return null;
    // ``page_type`` is optional — a See-also link emitted by the writer
    // for a bare ``[[slug]]`` (no type prefix) has no page_type. We still
    // surface a selection so the viewer renders the "no longer exists"
    // state instead of the "pick a page" prompt.
    const pageType = searchParams.get('page_type') ?? '';
    return { pageType, slug };
  }, [searchParams]);

  const select = useCallback(
    (next: SelectedArtifact | null) => {
      setSearchParams(
        (prev) => {
          const params = new URLSearchParams(prev);
          if (next) {
            if (next.pageType) {
              params.set('page_type', next.pageType);
            } else {
              params.delete('page_type');
            }
            params.set('slug', next.slug);
          } else {
            params.delete('page_type');
            params.delete('slug');
          }
          return params;
        },
        { replace: false },
      );
    },
    [setSearchParams],
  );

  return { selected, select };
}

/**
 * View switch between the markdown page viewer and the graph view.
 * Stored in the URL as ``?view=graph`` so the choice is bookmarkable
 * and browser back/forward swaps between views.
 */
export function useArtifactView() {
  const [searchParams, setSearchParams] = useSearchParams();

  const view: ArtifactView = useMemo(() => {
    return searchParams.get('view') === 'graph' ? 'graph' : 'page';
  }, [searchParams]);

  const setView = useCallback(
    (next: ArtifactView) => {
      setSearchParams(
        (prev) => {
          const params = new URLSearchParams(prev);
          if (next === 'graph') {
            params.set('view', 'graph');
          } else {
            params.delete('view');
          }
          return params;
        },
        { replace: false },
      );
    },
    [setSearchParams],
  );

  return { view, setView };
}
