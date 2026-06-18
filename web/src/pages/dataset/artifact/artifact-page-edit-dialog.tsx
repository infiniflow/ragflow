import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Textarea } from '@/components/ui/textarea';
import {
  useFetchDatasetArtifactPage,
  useUpdateDatasetArtifactPage,
} from '@/hooks/use-dataset-artifact-request';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Components } from 'react-markdown';
import ReactMarkdown from 'react-markdown';
import remarkBreaks from 'remark-breaks';
import remarkGfm from 'remark-gfm';
import { ArtifactLinkRenderer } from './artifact-link-renderer';

interface IProps {
  open: boolean;
  pageType: string | null;
  slug: string | null;
  onClose: () => void;
}

/**
 * Double-click-from-canvas edit dialog. Two panes:
 *
 *   left  — live markdown preview, rendered with the same components the
 *           regular ArtifactViewer uses (ReactMarkdown + GFM +
 *           ArtifactLinkRenderer for the artifact/ hrefs).
 *   right — raw textarea bound to the local `draft`.
 *
 * Save is "minimal" per the backend contract: only the page row is
 * updated. The canvas graph / per-entity / per-relation rows stay stale
 * until the next artifact compile.
 */
export function ArtifactPageEditDialog({
  open,
  pageType,
  slug,
  onClose,
}: IProps) {
  const { t } = useTranslation();
  const { data, loading } = useFetchDatasetArtifactPage(
    pageType ?? undefined,
    slug ?? undefined,
  );
  const { updatePage, loading: saving } = useUpdateDatasetArtifactPage();

  const initialContent = data?.content_md_rendered ?? '';
  const [draft, setDraft] = useState<string>('');
  const [dirty, setDirty] = useState<boolean>(false);

  // Seed the editor when a new page loads or the dialog re-opens. We don't
  // overwrite a dirty buffer — if the user has typed something we keep it.
  useEffect(() => {
    if (!open) return;
    if (!dirty) setDraft(initialContent);
  }, [open, initialContent, dirty]);

  // Reset dirty + draft on close so the next open is clean.
  useEffect(() => {
    if (!open) {
      setDirty(false);
      setDraft('');
    }
  }, [open]);

  const previewComponents = useMemo<Components>(
    () => ({
      a: ArtifactLinkRenderer,
    }),
    [],
  );

  const onChangeDraft = (next: string) => {
    setDraft(next);
    if (!dirty) setDirty(true);
  };

  const onRequestClose = () => {
    if (
      dirty &&
      !window.confirm(t('artifact.editDialog.discardConfirm') as string)
    ) {
      return;
    }
    onClose();
  };

  const onSave = async () => {
    if (!pageType || !slug) return;
    await updatePage({ pageType, slug, content_md: draft });
    setDirty(false);
    onClose();
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onRequestClose();
      }}
    >
      <DialogContent className="max-w-6xl w-[90vw] h-[80vh] flex flex-col p-0 gap-0">
        <DialogHeader className="px-6 py-4 border-b border-border-button">
          <DialogTitle className="text-base">
            {data?.title || slug || t('artifact.editDialog.title')}
          </DialogTitle>
        </DialogHeader>

        <div className="flex-1 min-h-0 grid grid-cols-2 divide-x divide-border-button">
          <section
            className="overflow-y-auto px-6 py-4"
            aria-label={t('artifact.editDialog.previewPaneLabel') as string}
          >
            {loading && !initialContent ? (
              <div className="text-text-secondary text-sm">
                {t('common.loading')}
              </div>
            ) : (
              <article className="prose dark:prose-invert max-w-none">
                <ReactMarkdown
                  remarkPlugins={[remarkGfm, remarkBreaks]}
                  components={previewComponents}
                >
                  {draft || initialContent}
                </ReactMarkdown>
              </article>
            )}
          </section>

          <section
            className="flex flex-col overflow-hidden"
            aria-label={t('artifact.editDialog.editorPaneLabel') as string}
          >
            <Textarea
              className="flex-1 min-h-0 resize-none rounded-none border-0 font-mono text-sm leading-6 focus-visible:ring-0 focus-visible:ring-offset-0"
              value={draft}
              onChange={(e) => onChangeDraft(e.target.value)}
              spellCheck={false}
              placeholder={t('artifact.editDialog.editorPlaceholder') as string}
              disabled={loading || saving}
            />
          </section>
        </div>

        <DialogFooter className="px-6 py-3 border-t border-border-button">
          <Button variant="ghost" onClick={onRequestClose} disabled={saving}>
            {t('common.cancel')}
          </Button>
          <Button
            onClick={onSave}
            disabled={!dirty || saving || !pageType || !slug}
          >
            {saving ? t('common.saving') : t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
