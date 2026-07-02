import Empty from '@/components/empty/empty';
import MarkdownEditor from '@/components/markdown-editor';
import { ReferenceDocumentList } from '@/components/next-message-item/reference-document-list';
import { Button } from '@/components/ui/button';
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from '@/components/ui/resizable';
import { Spin } from '@/components/ui/spin';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useFetchDocumentsByIds } from '@/hooks/use-document-request';
import {
  useFetchArtifactPage,
  useFetchWikiCommit,
} from '@/hooks/use-knowledge-request';
import { Docagg } from '@/interfaces/database/chat';
import { IArtifact, IWikiCommit } from '@/interfaces/database/dataset';
import { downloadMarkdownFile } from '@/utils/file-util';
import { Download } from 'lucide-react';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useCommitArtifact } from './hooks/use-commit-artifact';
import { useWikiEditor } from './hooks/use-wiki-editor';
import { VersionHistorySheet } from './version-history-sheet';
import { WikiCommitModal } from './wiki-commit-modal';
import { WikiVersionDiffPanel } from './wiki-version-diff-panel';

type WikiDetailContentProps = {
  selectedArtifact: IArtifact | null;
  selectedVersion: IWikiCommit | null;
  onSelectVersion: (version: IWikiCommit | null) => void;
};

export function WikiDetailContent({
  selectedArtifact,
  selectedVersion,
  onSelectVersion,
}: WikiDetailContentProps) {
  const { t } = useTranslation();
  const isVersionView = !!selectedVersion;

  const { data: pageData, loading: pageLoading } = useFetchArtifactPage(
    isVersionView ? null : selectedArtifact,
  );
  const { data: commitDetail, loading: commitLoading } = useFetchWikiCommit(
    selectedVersion?.id ?? null,
  );

  const title = pageData?.title ?? selectedArtifact?.title;

  const pageType = isVersionView
    ? selectedArtifact?.page_type
    : pageData?.page_type;
  const slug = isVersionView ? selectedArtifact?.slug : pageData?.slug;
  const content = isVersionView
    ? (commitDetail?.content_after ?? '')
    : (pageData?.content_md_rendered ?? '');
  const sourceDocIds = isVersionView ? [] : (pageData?.source_doc_ids ?? []);

  const editorKey = isVersionView
    ? `${selectedArtifact?.slug}@${selectedVersion?.id}`
    : selectedArtifact?.slug;

  const {
    editedContent,
    isDirty,
    handleContentChange,
    handleCancelEdit,
    handleMarkAsSaved,
  } = useWikiEditor({
    content,
    artifactSlug: editorKey,
  });

  const handleCommitSuccess = useCallback(() => {
    handleMarkAsSaved();
    if (isVersionView) {
      onSelectVersion(null);
    }
  }, [handleMarkAsSaved, isVersionView, onSelectVersion]);

  const { isOpen, open, close, form, handleConfirm, isUpdating } =
    useCommitArtifact({
      editedContent,
      pageType: pageType ?? '',
      slug: slug ?? '',
      onSuccess: handleCommitSuccess,
    });

  const { documents } = useFetchDocumentsByIds(sourceDocIds);

  const referenceDocuments = useMemo<Docagg[]>(() => {
    return documents.map(
      (doc): Docagg => ({
        doc_id: doc.id,
        doc_name: doc.name,
        count: 0,
      }),
    );
  }, [documents]);

  const handleExport = useCallback(() => {
    const filename = `${title ?? 'document'}.md`;
    downloadMarkdownFile(editedContent, filename);
  }, [title, editedContent]);

  const renderToolbarButtons = () => {
    if (isDirty) {
      return (
        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={handleCancelEdit}
          >
            {t('common.cancel')}
          </Button>
          <Button type="button" size="sm" onClick={open}>
            {t('knowledgeDetails.commit')}
          </Button>
        </div>
      );
    }

    return (
      <div className="flex items-center gap-1">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="size-8"
              onClick={handleExport}
            >
              <Download className="size-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('knowledgeDetails.export')}</TooltipContent>
        </Tooltip>
        <VersionHistorySheet
          selectedArtifact={selectedArtifact}
          selectedVersion={selectedVersion}
          onSelectVersion={onSelectVersion}
        />
      </div>
    );
  };

  const loading = isVersionView ? commitLoading : pageLoading;

  return (
    <section className="size-full min-w-0 flex flex-col">
      {selectedArtifact ? (
        <>
          <header className="shrink-0 px-8 pt-8 pb-4">
            <div className="flex items-start justify-between">
              <div className="flex flex-col gap-2">
                <h1 className="text-3xl font-semibold text-text-primary">
                  {title ?? selectedArtifact.title}
                </h1>
                <div className="flex items-center gap-2">
                  {isVersionView && commitDetail && (
                    <span className="text-sm text-accent-primary bg-accent-primary-5 px-2 py-0.5 rounded">
                      {commitDetail.title}
                    </span>
                  )}
                </div>
              </div>

              {renderToolbarButtons()}
            </div>
          </header>

          <div className="flex-1 min-h-0 flex flex-col border-t border-border-button">
            <ResizablePanelGroup direction="horizontal" className="flex-1">
              <ResizablePanel minSize={30}>
                <div className="h-full min-w-0 overflow-y-auto px-8 pb-8 flex flex-col">
                  {loading && !content ? (
                    <div className="py-8 flex justify-center">
                      <Spin size="large" />
                    </div>
                  ) : (
                    <MarkdownEditor
                      content={editedContent}
                      onChange={handleContentChange}
                    />
                  )}

                  {referenceDocuments.length > 0 && (
                    <div className="mt-8">
                      <h3 className="text-sm font-medium text-text-secondary mb-3">
                        {t('knowledgeDetails.sourceDocuments')}
                      </h3>
                      <ReferenceDocumentList list={referenceDocuments} />
                    </div>
                  )}
                </div>
              </ResizablePanel>

              {isVersionView && commitDetail && (
                <>
                  <ResizableHandle withHandle />
                  <ResizablePanel defaultSize={30} minSize={20}>
                    <WikiVersionDiffPanel
                      diff={commitDetail.diff}
                      title={commitDetail.comments || commitDetail.title}
                    />
                  </ResizablePanel>
                </>
              )}
            </ResizablePanelGroup>
          </div>

          <WikiCommitModal
            open={isOpen}
            onOpenChange={close}
            form={form}
            onConfirm={handleConfirm}
            loading={isUpdating}
          />
        </>
      ) : (
        <div className="flex-1 overflow-y-auto p-8">
          <Empty
            className="h-full"
            text={t('knowledgeDetails.selectArtifact')}
          />
        </div>
      )}
    </section>
  );
}
