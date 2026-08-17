import { useTranslation } from 'react-i18next';

import MarkdownEditor from '@/components/markdown-editor';
import { ReferenceDocumentList } from '@/components/next-message-item/reference-document-list';
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from '@/components/ui/resizable';
import { Spin } from '@/components/ui/spin';
import type { Docagg } from '@/interfaces/database/chat';
import type { IWikiCommitDetail } from '@/interfaces/database/dataset';

import type { WikiPageType } from './utils/parse-wiki-link';
import { WikiVersionDiffPanel } from './wiki-version-diff-panel';

type WikiDetailEditorPanelProps = {
  loading: boolean;
  editedContent: string;
  displayedContent: string | undefined;
  referenceDocuments: Docagg[];
  isVersionView: boolean;
  commitDetail: IWikiCommitDetail | null | undefined;
  onContentChange: (value: string) => void;
  onWikiLinkClick: (pageType: WikiPageType, slug: string) => void;
};

export function WikiDetailEditorPanel({
  loading,
  editedContent,
  displayedContent,
  referenceDocuments,
  isVersionView,
  commitDetail,
  onContentChange,
  onWikiLinkClick,
}: WikiDetailEditorPanelProps) {
  const { t } = useTranslation();

  return (
    <div className="flex-1 min-h-0 flex flex-col border-t border-border-button">
      <ResizablePanelGroup direction="horizontal" className="flex-1">
        <ResizablePanel minSize={30}>
          <div className="h-full min-w-0 overflow-y-auto px-8 pb-8 flex flex-col">
            {loading && !displayedContent ? (
              <div className="py-8 flex justify-center">
                <Spin size="large" />
              </div>
            ) : (
              <MarkdownEditor
                content={editedContent}
                onChange={onContentChange}
                onWikiLinkClick={onWikiLinkClick}
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
  );
}
