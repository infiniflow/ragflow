import Empty from '@/components/empty/empty';
import MarkdownEditor from '@/components/markdown-editor';
import { ReferenceDocumentList } from '@/components/next-message-item/reference-document-list';
import { Button } from '@/components/ui/button';
import { Spin } from '@/components/ui/spin';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useFetchDocumentsByIds } from '@/hooks/use-document-request';
import { useFetchArtifactPage } from '@/hooks/use-knowledge-request';
import { Docagg } from '@/interfaces/database/chat';
import { IArtifact } from '@/interfaces/database/dataset';
import { VersionHistorySheet } from '@/pages/dataset/compilation/version-history-sheet';
import { Upload } from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

type WikiDetailContentProps = {
  selectedArtifact: IArtifact | null;
};

export function WikiDetailContent({
  selectedArtifact,
}: WikiDetailContentProps) {
  const { t } = useTranslation();
  const { data, loading } = useFetchArtifactPage(selectedArtifact);
  const { documents } = useFetchDocumentsByIds(data?.source_doc_ids ?? []);

  const referenceDocuments = useMemo<Docagg[]>(() => {
    return documents.map(
      (doc): Docagg => ({
        doc_id: doc.id,
        doc_name: doc.name,
        count: 0,
      }),
    );
  }, [documents]);

  return (
    <section className="size-full min-w-0 flex flex-col">
      {selectedArtifact ? (
        <>
          <header className="shrink-0 px-8 pt-8 pb-4">
            <div className="flex items-start justify-between">
              <div className="flex items-center gap-3">
                <h1 className="text-3xl font-semibold text-text-primary">
                  {data?.title ?? selectedArtifact.title}
                </h1>
                {data?.page_type && (
                  <span className="text-sm text-state-success bg-state-success/10 px-2 py-0.5 rounded uppercase">
                    {data.page_type}
                  </span>
                )}
              </div>

              <div className="flex items-center gap-1">
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button variant="ghost" size="icon" className="size-8">
                      <Upload className="size-4" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>
                    {t('knowledgeDetails.export')}
                  </TooltipContent>
                </Tooltip>
                <VersionHistorySheet />
              </div>
            </div>
          </header>

          <div className="flex-1 overflow-y-auto px-8 pb-8 flex flex-col">
            {loading && !data ? (
              <div className="py-8 flex justify-center">
                <Spin size="large" />
              </div>
            ) : (
              <MarkdownEditor content={data?.content_md_rendered ?? ''} />
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
