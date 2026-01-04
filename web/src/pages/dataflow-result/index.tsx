import DocumentPreview from '@/components/document-preview';
import { useFetchNextChunkList } from '@/hooks/use-chunk-request';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useFetchPipelineFileLogDetail,
  useFetchPipelineResult,
  useGetChunkHighlights,
  useGetPipelineResultSearchParams,
  useHandleChunkCardClick,
  useRerunDataflow,
  useSummaryInfo,
  useTimelineDataFlow,
} from './hooks';

import DocumentHeader from '@/components/document-preview/document-header';

import { useGetDocumentUrl } from '@/components/document-preview/hooks';
import { TimelineNode } from '@/components/originui/timeline';
import { PageHeader } from '@/components/page-header';
import Spotlight from '@/components/spotlight';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import { Button } from '@/components/ui/button';
import { Modal } from '@/components/ui/modal/modal';
import { AgentCategory } from '@/constants/agent';
import { Images } from '@/constants/common';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useGetKnowledgeSearchParams } from '@/hooks/route-hook';
import TimelineDataFlow from './components/time-line';
import { TimelineNodeType } from './constant';
import styles from './index.module.less';
import { IDslComponent, IPipelineFileLogDetail } from './interface';
import ParserContainer from './parser';

const Chunk = () => {
  const { isReadOnly, knowledgeId, agentId, agentTitle, documentExtension } =
    useGetPipelineResultSearchParams();

  const isAgent = !!agentId;

  const { pipelineResult } = useFetchPipelineResult({ agentId });

  const {
    data: { documentInfo },
  } = useFetchNextChunkList(!isAgent);

  const { selectedChunk, handleChunkCardClick } = useHandleChunkCardClick();
  const [activeStepId, setActiveStepId] = useState<number | string>(2);
  const { data: dataset } = useFetchPipelineFileLogDetail({
    isAgent,
  });
  const { t } = useTranslation();

  const { timelineNodes } = useTimelineDataFlow(
    agentId ? (pipelineResult as IPipelineFileLogDetail) : dataset,
  );

  const {
    navigateToDatasetOverview,
    navigateToDatasetList,
    navigateToAgents,
    navigateToAgent,
  } = useNavigatePage();
  let fileUrl = useGetDocumentUrl(isAgent);

  const { highlights, setWidthAndHeight } =
    useGetChunkHighlights(selectedChunk);

  const fileType = useMemo(() => {
    if (isAgent) {
      return Images.some((x) => x === documentExtension)
        ? documentInfo?.name.split('.').pop() || 'visual'
        : documentExtension;
    }
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
  }, [documentExtension, documentInfo?.name, documentInfo?.type, isAgent]);

  const {
    handleReRunFunc,
    isChange,
    setIsChange,
    loading: reRunLoading,
  } = useRerunDataflow({
    data: dataset,
  });

  const handleStepChange = (id: number | string, step: TimelineNode) => {
    if (isChange) {
      Modal.show({
        visible: true,
        className: '!w-[560px]',
        title: t('dataflowParser.changeStepModalTitle'),
        children: (
          <div
            className="text-sm text-text-secondary"
            dangerouslySetInnerHTML={{
              __html: t('dataflowParser.changeStepModalContent', {
                step: step?.title,
              }),
            }}
          ></div>
        ),
        onVisibleChange: () => {
          Modal.destroy();
        },
        footer: (
          <div className="flex justify-end gap-2">
            <Button variant={'outline'} onClick={() => Modal.destroy()}>
              {t('dataflowParser.changeStepModalCancelText')}
            </Button>
            <Button
              variant={'secondary'}
              className="!bg-state-error text-text-primary"
              onClick={() => {
                Modal.destroy();
                setActiveStepId(id);
                setIsChange(false);
              }}
            >
              {t('dataflowParser.changeStepModalConfirmText')}
            </Button>
          </div>
        ),
      });
    } else {
      setActiveStepId(id);
    }
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
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink
                onClick={() => {
                  if (knowledgeId) {
                    navigateToDatasetList();
                  }
                  if (agentId) {
                    navigateToAgents();
                  }
                }}
              >
                {knowledgeId ? t('knowledgeDetails.dataset') : t('header.flow')}
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbLink
                onClick={() => {
                  if (knowledgeId) {
                    navigateToDatasetOverview(knowledgeId)();
                  }
                  if (isAgent) {
                    navigateToAgent(agentId, AgentCategory.DataflowCanvas)();
                  }
                }}
              >
                {knowledgeId ? t('knowledgeDetails.overview') : agentTitle}
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>
                {knowledgeId ? documentInfo?.name : t('flow.viewResult')}
              </BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
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
              currentTimeNode?.type === TimelineNodeType.characterSplitter ||
              currentTimeNode?.type === TimelineNodeType.titleSplitter ||
              currentTimeNode?.type === TimelineNodeType.contextGenerator) && (
              <ParserContainer
                isReadonly={isReadOnly}
                isChange={isChange}
                reRunLoading={reRunLoading}
                setIsChange={setIsChange}
                step={currentTimeNode as TimelineNode}
                data={
                  currentTimeNode.detail as {
                    value: IDslComponent;
                    key: string;
                  }
                }
                summaryInfo={summaryInfo}
                clickChunk={handleChunkCardClick}
                reRunFunc={handleReRunFunc}
              />
            )}
            {/* )} */}
            <Spotlight opcity={0.6} coverage={60} />
          </div>
        </div>
      </div>
    </>
  );
};

export default Chunk;
