import { useFetchNextChunkList } from '@/hooks/use-chunk-request';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import DocumentPreview from './components/document-preview';
import {
  useGetChunkHighlights,
  useHandleChunkCardClick,
  useRerunDataflow,
} from './hooks';

import DocumentHeader from './components/document-preview/document-header';

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
import {
  QueryStringMap,
  useNavigatePage,
} from '@/hooks/logic-hooks/navigate-hooks';
import { useGetKnowledgeSearchParams } from '@/hooks/route-hook';
import { useFetchKnowledgeBaseConfiguration } from '@/hooks/use-knowledge-request';
import { ChunkerContainer } from './chunker';
import { useGetDocumentUrl } from './components/document-preview/hooks';
import TimelineDataFlow, {
  TimelineNodeArr,
  TimelineNodeType,
} from './components/time-line';
import styles from './index.less';
import ParserContainer from './parser';

const Chunk = () => {
  const {
    data: { documentInfo },
  } = useFetchNextChunkList();
  const { selectedChunkId } = useHandleChunkCardClick();
  const [activeStepId, setActiveStepId] = useState<number | string>(0);
  const { data: dataset } = useFetchKnowledgeBaseConfiguration();
  const { isChange, setIsChange } = useRerunDataflow();
  const { t } = useTranslation();

  const { navigateToDataset, getQueryString, navigateToDatasetList } =
    useNavigatePage();
  const fileUrl = useGetDocumentUrl();

  const { highlights, setWidthAndHeight } =
    useGetChunkHighlights(selectedChunkId);

  const fileType = useMemo(() => {
    switch (documentInfo?.type) {
      case 'doc':
        return documentInfo?.name.split('.').pop() || 'doc';
      case 'visual':
      case 'docx':
      case 'txt':
      case 'md':
      case 'pdf':
        return documentInfo?.type;
    }
    return 'unknown';
  }, [documentInfo]);

  const handleStepChange = (id: number | string, step: TimelineNode) => {
    console.log(id, step);
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
          Modal.hide();
        },
        footer: (
          <div className="flex justify-end gap-2">
            <Button variant={'outline'} onClick={() => Modal.hide()}>
              {t('dataflowParser.changeStepModalCancelText')}
            </Button>
            <Button
              variant={'secondary'}
              className="!bg-state-error text-text-primary"
              onClick={() => {
                Modal.hide();
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
      TimelineNodeArr.find((node) => node.id === activeStepId) ||
      ({} as TimelineNode)
    );
  }, [activeStepId]);
  return (
    <>
      <PageHeader>
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink onClick={navigateToDatasetList}>
                {t('knowledgeDetails.dataset')}
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbLink
                onClick={navigateToDataset(
                  getQueryString(QueryStringMap.id) as string,
                )}
              >
                {dataset.name}
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>{documentInfo?.name}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
      </PageHeader>
      {type === 'dataflow' && (
        <div className=" absolute ml-[50%] translate-x-[-50%] top-4 flex justify-center">
          <TimelineDataFlow
            activeFunc={handleStepChange}
            activeId={activeStepId}
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
          <div className="h-dvh border-r -mt-3"></div>
          <div className="w-3/5 h-full">
            {currentTimeNode?.type === TimelineNodeType.chunk && (
              <ChunkerContainer
                isChange={isChange}
                setIsChange={setIsChange}
                step={currentTimeNode as TimelineNode}
              />
            )}
            {currentTimeNode?.type === TimelineNodeType.parser && (
              <ParserContainer
                isChange={isChange}
                setIsChange={setIsChange}
                step={currentTimeNode as TimelineNode}
              />
            )}
            <Spotlight opcity={0.6} coverage={60} />
          </div>
        </div>
      </div>
    </>
  );
};

export default Chunk;
