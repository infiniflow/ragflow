import { useFetchDocumentsByIds } from '@/hooks/use-document-request';
import {
  useFetchArtifactPage,
  useFetchWikiCommit,
} from '@/hooks/use-knowledge-request';
import { Docagg } from '@/interfaces/database/chat';
import { IArtifact, IWikiCommit } from '@/interfaces/database/dataset';
import { downloadMarkdownFile } from '@/utils/file-util';
import { useCallback, useEffect, useMemo } from 'react';

import type { WikiPageType } from '../utils/parse-wiki-link';
import { useCommitArtifact } from './use-commit-artifact';
import { useWikiEditor } from './use-wiki-editor';
import { useWikiLinkNavigation } from './use-wiki-link-navigation';

type UseWikiDetailContentOptions = {
  selectedArtifact: IArtifact | null;
  selectedVersion: IWikiCommit | null;
  onSelectVersion: (version: IWikiCommit | null) => void;
  onSelectArtifact: (artifact: IArtifact) => void;
};

export function useWikiDetailContent({
  selectedArtifact,
  selectedVersion,
  onSelectVersion,
  onSelectArtifact,
}: UseWikiDetailContentOptions) {
  const isVersionView = !!selectedVersion;

  const { data: pageData, loading: pageLoading } = useFetchArtifactPage(
    isVersionView ? null : selectedArtifact,
  );
  const { data: commitDetail, loading: commitLoading } = useFetchWikiCommit(
    selectedVersion?.id ?? null,
  );

  const {
    currentEntry,
    previousEntry,
    canGoBack,
    push,
    goBack,
    reset,
    updateCurrentTitle,
  } = useWikiLinkNavigation();

  // Keep the current stack entry's title in sync with pageData.
  useEffect(() => {
    if (!currentEntry || !pageData) return;
    if (currentEntry.slug !== pageData.slug) return;
    updateCurrentTitle(pageData.title);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentEntry?.slug, pageData?.slug, pageData?.title, updateCurrentTitle]);

  // When selectedArtifact changes from the left panel (not from our own
  // wiki-link navigation), reset the stack.  Wiki-link clicks call push()
  // before onSelectArtifact(), so currentEntry.slug already matches by the
  // time this effect runs and we bail out.
  useEffect(() => {
    if (isVersionView || !selectedArtifact) return;
    if (currentEntry?.slug === selectedArtifact.slug) return;

    reset({
      slug: selectedArtifact.slug,
      title: selectedArtifact.title,
      pageType: selectedArtifact.page_type ?? '',
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    isVersionView,
    selectedArtifact?.slug,
    selectedArtifact?.title,
    selectedArtifact?.page_type,
    currentEntry?.slug,
    reset,
  ]);

  const content = isVersionView
    ? (commitDetail?.content_after ?? '')
    : (pageData?.content_md_rendered ?? '');

  const title =
    currentEntry?.title ||
    pageData?.title ||
    selectedArtifact?.title ||
    currentEntry?.slug ||
    '';

  const displayedArtifact = currentEntry
    ? { slug: currentEntry.slug, title: currentEntry.title, page_type: currentEntry.pageType }
    : selectedArtifact;

  const previousEntryTitle = previousEntry?.title || previousEntry?.slug;

  const editorKey = isVersionView
    ? `${selectedArtifact?.slug}@${selectedVersion?.id}`
    : (selectedArtifact?.slug ?? '');

  const handleMarkdownLinkClick = useCallback(
    (pageType: WikiPageType, slug: string) => {
      if (isVersionView) return;
      if (currentEntry?.slug === slug && currentEntry?.pageType === pageType)
        return;

      // Sync the current entry's title from pageData before pushing a new
      // entry, so the title is preserved when this entry becomes the
      // "previous" entry for the back button.
      if (
        currentEntry &&
        pageData &&
        pageData.slug === currentEntry.slug &&
        pageData.title
      ) {
        updateCurrentTitle(pageData.title);
      }

      push({ slug, title: '', pageType });
      onSelectArtifact({ slug, page_type: pageType, title: '' });
    },
    [
      push,
      onSelectArtifact,
      isVersionView,
      currentEntry,
      pageData,
      updateCurrentTitle,
    ],
  );

  const handleBack = useCallback(() => {
    if (!previousEntry) return;
    goBack();
    onSelectArtifact({
      slug: previousEntry.slug,
      page_type: previousEntry.pageType,
      title: previousEntry.title,
    });
  }, [goBack, previousEntry, onSelectArtifact]);

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
      pageType: currentEntry?.pageType ?? selectedArtifact?.page_type ?? '',
      slug: currentEntry?.slug ?? selectedArtifact?.slug ?? '',
      onSuccess: handleCommitSuccess,
    });

  const { documents } = useFetchDocumentsByIds(
    pageData?.source_doc_ids ?? [],
  );

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

  const loading = isVersionView ? commitLoading : pageLoading;

  return {
    isVersionView,
    title,
    displayedArtifact,
    displayedContent: content,
    displayedPageType: currentEntry?.pageType ?? pageData?.page_type ?? '',
    displayedSlug: currentEntry?.slug ?? pageData?.slug ?? '',
    selectedArtifact,
    selectedVersion,
    commitDetail,
    canGoBack,
    previousEntry,
    previousEntryTitle,
    linkNavLoading: false,
    loading,
    editedContent,
    isDirty,
    referenceDocuments,
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
    onSelectVersion,
  };
}
