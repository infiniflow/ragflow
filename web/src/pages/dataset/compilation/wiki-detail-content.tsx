import { useTranslation } from 'react-i18next';

import Empty from '@/components/empty/empty';
import { IArtifact, IWikiCommit } from '@/interfaces/database/dataset';

import { useWikiDetailContent } from './hooks/use-wiki-detail-content';
import { WikiCommitModal } from './wiki-commit-modal';
import { WikiDetailEditorPanel } from './wiki-detail-editor-panel';
import { WikiDetailHeader } from './wiki-detail-header';
import { WikiDetailToolbar } from './wiki-detail-toolbar';

type WikiDetailContentProps = {
  selectedArtifact: IArtifact | null;
  selectedVersion: IWikiCommit | null;
  onSelectVersion: (version: IWikiCommit | null) => void;
  onSelectArtifact: (artifact: IArtifact) => void;
};

export function WikiDetailContent({
  selectedArtifact,
  selectedVersion,
  onSelectVersion,
  onSelectArtifact,
}: WikiDetailContentProps) {
  const { t } = useTranslation();
  const {
    isVersionView,
    title,
    displayedArtifact,
    commitDetail,
    canGoBack,
    previousEntryTitle,
    linkNavLoading,
    loading,
    editedContent,
    displayedContent,
    referenceDocuments,
    isDirty,
    isOpen,
    open,
    close,
    form,
    handleConfirm,
    isUpdating,
    handleCancelEdit,
    handleContentChange,
    handleMarkdownLinkClick,
    handleBack,
    handleExport,
  } = useWikiDetailContent({
    selectedArtifact,
    selectedVersion,
    onSelectVersion,
    onSelectArtifact,
  });

  const toolbar = (
    <WikiDetailToolbar
      isDirty={isDirty}
      selectedArtifact={selectedArtifact}
      selectedVersion={selectedVersion}
      onCancelEdit={handleCancelEdit}
      onCommitClick={open}
      onExport={handleExport}
      onSelectVersion={onSelectVersion}
    />
  );

  return (
    <section className="size-full min-w-0 flex flex-col">
      {selectedArtifact ? (
        <>
          <WikiDetailHeader
            title={title}
            displayedArtifact={displayedArtifact}
            commitDetail={commitDetail}
            isVersionView={isVersionView}
            toolbar={toolbar}
            canGoBack={canGoBack}
            previousEntryTitle={previousEntryTitle}
            linkNavLoading={linkNavLoading}
            onBack={handleBack}
          />

          <WikiDetailEditorPanel
            loading={loading}
            editedContent={editedContent}
            displayedContent={displayedContent}
            referenceDocuments={referenceDocuments}
            isVersionView={isVersionView}
            commitDetail={commitDetail}
            onContentChange={handleContentChange}
            onWikiLinkClick={handleMarkdownLinkClick}
          />

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
