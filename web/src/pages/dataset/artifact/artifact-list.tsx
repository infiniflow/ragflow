import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  useClearDatasetArtifacts,
  useListDatasetArtifacts,
} from '@/hooks/use-dataset-artifact-request';
import { ArtifactListItem } from '@/interfaces/database/dataset-artifact';
import { cn } from '@/lib/utils';
import { Network, Trash2 } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useArtifactSelection,
  useArtifactView,
} from './hooks/use-artifact-state';

/**
 * Map a stored ``slug_kwd`` (which is the full ``<page_type>/<slug-tail>``)
 * back to its tail. The selection state stores only the tail because the
 * markdown link format is also ``artifact/<kb_id>/<page_type>/<tail>``.
 */
function slugTail(fullSlug: string, pageType: string): string {
  const prefix = `${pageType}/`;
  return fullSlug.startsWith(prefix) ? fullSlug.slice(prefix.length) : fullSlug;
}

function pageTypeBadgeVariant(
  pageType: string,
): 'default' | 'secondary' | 'outline' {
  switch (pageType) {
    case 'entity':
      return 'default';
    case 'concept':
      return 'secondary';
    case 'topic':
      return 'outline';
    default:
      return 'outline';
  }
}

const DEFAULT_PAGE_SIZE = 20;

export function ArtifactList() {
  const { t } = useTranslation();
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);

  const { selected, select } = useArtifactSelection();
  const { view, setView } = useArtifactView();
  // Backend orders by outlinks_int desc → most-connected pages first.
  const { data, loading } = useListDatasetArtifacts({ page, pageSize });
  const { clearArtifacts, loading: clearing } = useClearDatasetArtifacts();

  const handleClear = useCallback(async () => {
    const code = await clearArtifacts();
    if (code === 0) {
      // The selected page no longer exists — drop the URL state so the
      // viewer falls back to the "Select a page" prompt instead of a
      // "no longer exists" message for a stale slug.
      select(null);
      setPage(1);
    }
  }, [clearArtifacts, select]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return data.items;
    // Local filter only applies to the *current* server page. Acceptable
    // for normal KB sizes; if a KB ever has thousands of pages we'd push
    // the search to the server, but that's not the current shape.
    return data.items.filter(
      (item: ArtifactListItem) =>
        item.title.toLowerCase().includes(q) ||
        item.page_type.toLowerCase().includes(q) ||
        item.slug.toLowerCase().includes(q),
    );
  }, [data.items, search]);

  const handlePageChange = useCallback(
    (nextPage: number, nextPageSize: number) => {
      setPage(nextPage);
      setPageSize(nextPageSize);
    },
    [],
  );

  const handleSearchChange = useCallback((value: string) => {
    setSearch(value);
    // Reset to first page when the search term changes so the user
    // doesn't end up on an empty page-4 because filtering shrank the
    // visible row count.
    setPage(1);
  }, []);

  return (
    <div className="flex flex-col h-full min-w-0 border-r border-border-button">
      <header className="p-3 border-b border-border-button flex flex-col gap-2">
        <div className="flex items-center justify-between gap-2">
          <h3 className="text-sm font-medium text-text-primary flex items-center gap-2">
            <span>
              {t('artifact.pages')}{' '}
              <span className="text-text-secondary">({data.total})</span>
            </span>
            <Button
              variant={view === 'graph' ? 'secondary' : 'ghost'}
              size="sm"
              onClick={() => setView(view === 'graph' ? 'page' : 'graph')}
              aria-label={t('artifact.openGraph')}
              aria-pressed={view === 'graph'}
            >
              <Network className="size-3.5" />
              {t('artifact.graph')}
            </Button>
          </h3>
          <ConfirmDeleteDialog
            onOk={handleClear}
            title={t('artifact.clearAllTitle')}
            content={{
              title: t('artifact.clearAllConfirm'),
              node: (
                <p className="text-sm text-text-secondary">
                  {t('artifact.clearAllBody')}
                </p>
              ),
            }}
          >
            <Button
              variant="ghost"
              size="sm"
              disabled={clearing || data.total === 0}
              aria-label={t('artifact.clearAll')}
            >
              <Trash2 className="size-3.5" />
              {t('artifact.clearAll')}
            </Button>
          </ConfirmDeleteDialog>
        </div>
        <SearchInput
          value={search}
          onChange={(e) => handleSearchChange(e.target.value)}
          placeholder={t('common.search')}
        />
      </header>
      <div className="flex-1 overflow-auto">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('artifact.title')}</TableHead>
              <TableHead className="w-32">{t('artifact.pageType')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading && filtered.length === 0 && (
              <TableRow>
                <TableCell
                  colSpan={2}
                  className="text-center text-text-secondary py-6"
                >
                  {t('common.loading')}
                </TableCell>
              </TableRow>
            )}
            {!loading && filtered.length === 0 && (
              <TableRow>
                <TableCell
                  colSpan={2}
                  className="text-center text-text-secondary py-6"
                >
                  {search ? t('artifact.noMatches') : t('artifact.empty')}
                </TableCell>
              </TableRow>
            )}
            {filtered.map((item) => {
              const tail = slugTail(item.slug, item.page_type);
              const isSelected =
                selected?.pageType === item.page_type &&
                selected?.slug === tail;
              return (
                <TableRow
                  key={item.slug}
                  role="button"
                  tabIndex={0}
                  data-selected={isSelected || undefined}
                  className={cn('cursor-pointer', isSelected && 'bg-bg-card')}
                  onClick={() =>
                    select({ pageType: item.page_type, slug: tail })
                  }
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault();
                      select({ pageType: item.page_type, slug: tail });
                    }
                  }}
                >
                  <TableCell className="truncate max-w-0" title={item.title}>
                    {item.title}
                  </TableCell>
                  <TableCell>
                    <Badge variant={pageTypeBadgeVariant(item.page_type)}>
                      {item.page_type}
                    </Badge>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </div>
      {data.total > 0 ? (
        <footer className="border-t border-border-button p-2 flex justify-end">
          <RAGFlowPagination
            current={page}
            pageSize={pageSize}
            total={data.total}
            onChange={handlePageChange}
          />
        </footer>
      ) : null}
    </div>
  );
}
