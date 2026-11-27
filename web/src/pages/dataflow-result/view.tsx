import DocumentPreview from '@/components/document-preview';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useGetChunkHighlights,
  useHandleChunkCardClick,
  useSummaryInfo,
  useTimelineDataFlow,
} from './hooks';

import DocumentHeader from '@/components/document-preview/document-header';

import { TimelineNode } from '@/components/originui/timeline';
import { PageHeader } from '@/components/page-header';
import Spotlight from '@/components/spotlight';
import { Button } from '@/components/ui/button';
import { useGetKnowledgeSearchParams } from '@/hooks/route-hook';
import { IKnowledgeFile } from '@/interfaces/database/dataset';
import { LucideArrowBigLeft } from 'lucide-react';
import TimelineDataFlow from './components/time-line';
import { TimelineNodeType } from './constant';
import styles from './index.module.less';
import { IDslComponent, IPipelineFileLogDetail } from './interface';
import ParserContainer from './parser';

export function PipelineResultView({
  documentInfo,
  dataset,
  fileUrl,
  isReadOnly,
  backTo,
  documentExtension,
}: {
  documentInfo: IKnowledgeFile;
  dataset: IPipelineFileLogDetail;
  fileUrl: string;
  isReadOnly: boolean;
  backTo: string;
  documentExtension?: string;
}) {
  const { selectedChunk, handleChunkCardClick } = useHandleChunkCardClick();
  const [activeStepId, setActiveStepId] = useState<number | string>(2);
  const { t } = useTranslation();

  const { timelineNodes } = useTimelineDataFlow(dataset);

  const { highlights, setWidthAndHeight } =
    useGetChunkHighlights(selectedChunk);

  const fileType = useMemo(() => {
    if (documentExtension) return documentExtension;
    switch (documentInfo?.type) {
      case 'doc':
        return documentInfo?.name.split('.').pop() || 'doc';
      case 'visual':
        return documentInfo?.name.split('.').pop() || 'visual';
      case 'docx':
      case 'txt':
      case 'md':
      case 'mdx':
      case 'pdf':
        return documentInfo?.type;
    }
    return 'unknown';
  }, [documentExtension, documentInfo?.name, documentInfo?.type]);

  const handleStepChange = (id: number | string) => {
    setActiveStepId(id);
  };

  const { type } = useGetKnowledgeSearchParams();

  const currentTimeNode: TimelineNode = useMemo(() => {
    return (
      timelineNodes.find((node) => node.id === activeStepId) ||
      ({} as TimelineNode)
    );
  }, [activeStepId, timelineNodes]);
  const { summaryInfo } = useSummaryInfo(dataset, currentTimeNode);
  return (
    <>
      <PageHeader>
        <Button asLink variant="outline" to={backTo}>
          <LucideArrowBigLeft />
          {t('common.back')}
        </Button>
      </PageHeader>

      {type === 'dataflow' && (
        <div className=" absolute ml-[50%] translate-x-[-50%] top-4 flex justify-center">
          <TimelineDataFlow
            activeFunc={handleStepChange}
            activeId={activeStepId}
            data={dataset}
            timelineNodes={timelineNodes}
          />
        </div>
      )}
      <div className={styles.chunkPage}>
        <div className="flex flex-none gap-8 border border-border mt-[26px] p-3 rounded-lg h-[calc(100vh-100px)]">
          <div className="w-2/5">
            <div className="h-[50px] flex flex-col justify-end pb-[5px]">
              <DocumentHeader {...documentInfo} />
            </div>
            <section className={styles.documentPreview}>
              <DocumentPreview
                className={styles.documentPreview}
                fileType={fileType}
                highlights={highlights}
                setWidthAndHeight={setWidthAndHeight}
                url={fileUrl}
              ></DocumentPreview>
            </section>
          </div>
          <div className="h-[calc(100vh-100px)] border-r -mt-3"></div>
          <div className="w-3/5 h-full">
            {/* {currentTimeNode?.type === TimelineNodeType.splitter && (
              <ChunkerContainer
                isChange={isChange}
                setIsChange={setIsChange}
                step={currentTimeNode as TimelineNode}
              />
            )} */}
            {/* {currentTimeNode?.type === TimelineNodeType.parser && ( */}
            {(currentTimeNode?.type === TimelineNodeType.parser ||
              currentTimeNode?.type === TimelineNodeType.tokenChunker ||
              currentTimeNode?.type === TimelineNodeType.titleChunker ||
              currentTimeNode?.type === TimelineNodeType.contextGenerator) && (
              <ParserContainer
                isReadonly={isReadOnly}
                step={currentTimeNode as TimelineNode}
                data={
                  currentTimeNode.detail as {
                    value: IDslComponent;
                    key: string;
                  }
                }
                summaryInfo={summaryInfo}
                clickChunk={handleChunkCardClick}
              />
            )}
            {/* )} */}
            <Spotlight opcity={0.6} coverage={60} />
          </div>
        </div>
      </div>
    </>
  );
}
