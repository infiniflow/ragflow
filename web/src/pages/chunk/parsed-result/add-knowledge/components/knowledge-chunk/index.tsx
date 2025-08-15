import {
  useFetchNextChunkList,
  useSwitchChunk,
} from '@/hooks/use-chunk-request';
import classNames from 'classnames';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import ChunkCard from './components/chunk-card';
import CreatingModal from './components/chunk-creating-modal';
import DocumentPreview from './components/document-preview';
import {
  useChangeChunkTextMode,
  useDeleteChunkByIds,
  useGetChunkHighlights,
  useHandleChunkCardClick,
  useUpdateChunk,
} from './hooks';

import ChunkResultBar from './components/chunk-result-bar';
import CheckboxSets from './components/chunk-result-bar/checkbox-sets';
import DocumentHeader from './components/document-preview/document-header';

import { PageHeader } from '@/components/page-header';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import message from '@/components/ui/message';
import {
  RAGFlowPagination,
  RAGFlowPaginationType,
} from '@/components/ui/ragflow-pagination';
import { Spin } from '@/components/ui/spin';
import {
  QueryStringMap,
  useNavigatePage,
} from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchKnowledgeBaseConfiguration } from '@/hooks/use-knowledge-request';
import { useGetDocumentUrl } from '../../../knowledge-chunk/components/document-preview/hooks';
import styles from './index.less';

const Chunk = () => {
  const [selectedChunkIds, setSelectedChunkIds] = useState<string[]>([]);
  const { removeChunk } = useDeleteChunkByIds();
  const {
    data: { documentInfo, data = [], total },
    pagination,
    loading,
    searchString,
    handleInputChange,
    available,
    handleSetAvailable,
  } = useFetchNextChunkList();
  const { handleChunkCardClick, selectedChunkId } = useHandleChunkCardClick();
  const isPdf = documentInfo?.type === 'pdf';
  const { data: dataset } = useFetchKnowledgeBaseConfiguration();

  const { t } = useTranslation();
  const { changeChunkTextMode, textMode } = useChangeChunkTextMode();
  const { switchChunk } = useSwitchChunk();
  const [chunkList, setChunkList] = useState(data);
  const {
    chunkUpdatingLoading,
    onChunkUpdatingOk,
    showChunkUpdatingModal,
    hideChunkUpdatingModal,
    chunkId,
    chunkUpdatingVisible,
    documentId,
  } = useUpdateChunk();
  const { navigateToDataset, getQueryString, navigateToDatasetList } =
    useNavigatePage();
  const fileUrl = useGetDocumentUrl();
  useEffect(() => {
    setChunkList(data);
  }, [data]);
  const onPaginationChange: RAGFlowPaginationType['onChange'] = (
    page,
    size,
  ) => {
    setSelectedChunkIds([]);
    pagination.onChange?.(page, size);
  };

  const selectAllChunk = useCallback(
    (checked: boolean) => {
      setSelectedChunkIds(checked ? data.map((x) => x.chunk_id) : []);
    },
    [data],
  );

  const handleSingleCheckboxClick = useCallback(
    (chunkId: string, checked: boolean) => {
      setSelectedChunkIds((previousIds) => {
        const idx = previousIds.findIndex((x) => x === chunkId);
        const nextIds = [...previousIds];
        if (checked && idx === -1) {
          nextIds.push(chunkId);
        } else if (!checked && idx !== -1) {
          nextIds.splice(idx, 1);
        }
        return nextIds;
      });
    },
    [],
  );

  const showSelectedChunkWarning = useCallback(() => {
    message.warning(t('message.pleaseSelectChunk'));
  }, [t]);

  const handleRemoveChunk = useCallback(async () => {
    if (selectedChunkIds.length > 0) {
      const resCode: number = await removeChunk(selectedChunkIds, documentId);
      if (resCode === 0) {
        setSelectedChunkIds([]);
      }
    } else {
      showSelectedChunkWarning();
    }
  }, [selectedChunkIds, documentId, removeChunk, showSelectedChunkWarning]);

  const handleSwitchChunk = useCallback(
    async (available?: number, chunkIds?: string[]) => {
      let ids = chunkIds;
      if (!chunkIds) {
        ids = selectedChunkIds;
        if (selectedChunkIds.length === 0) {
          showSelectedChunkWarning();
          return;
        }
      }

      const resCode: number = await switchChunk({
        chunk_ids: ids,
        available_int: available,
        doc_id: documentId,
      });
      if (ids?.length && resCode === 0) {
        chunkList.forEach((x: any) => {
          if (ids.indexOf(x['chunk_id']) > -1) {
            x['available_int'] = available;
          }
        });
        setChunkList(chunkList);
      }
    },
    [
      switchChunk,
      documentId,
      selectedChunkIds,
      showSelectedChunkWarning,
      chunkList,
    ],
  );

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
              <BreadcrumbPage>{documentInfo.name}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
      </PageHeader>
      <div className={styles.chunkPage}>
        <div className="flex flex-1 gap-8">
          <div className="w-2/5">
            <div className="h-[100px] flex flex-col justify-end pb-[5px]">
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
          <div
            className={classNames(
              { [styles.pagePdfWrapper]: isPdf },
              'flex flex-col w-3/5',
            )}
          >
            <Spin spinning={loading} className={styles.spin} size="large">
              <div className="h-[100px] flex flex-col justify-end pb-[5px]">
                <div>
                  <h2 className="text-[24px]">{t('chunk.chunkResult')}</h2>
                  <div className="text-[14px] text-[#979AAB]">
                    {t('chunk.chunkResultTip')}
                  </div>
                </div>
              </div>
              <div className=" rounded-[16px] bg-[#FFF]/10 pl-[20px] pb-[20px] pt-[20px] box-border	mb-2">
                <ChunkResultBar
                  handleInputChange={handleInputChange}
                  searchString={searchString}
                  changeChunkTextMode={changeChunkTextMode}
                  createChunk={showChunkUpdatingModal}
                  available={available}
                  selectAllChunk={selectAllChunk}
                  handleSetAvailable={handleSetAvailable}
                />
                <div className="pt-[5px] pb-[5px]">
                  <CheckboxSets
                    selectAllChunk={selectAllChunk}
                    switchChunk={handleSwitchChunk}
                    removeChunk={handleRemoveChunk}
                    checked={selectedChunkIds.length === data.length}
                  />
                </div>
                <div className={styles.pageContent}>
                  <div
                    className={classNames(
                      styles.chunkContainer,
                      {
                        [styles.chunkOtherContainer]: !isPdf,
                      },
                      'flex flex-col gap-4',
                    )}
                  >
                    {chunkList.map((item) => (
                      <ChunkCard
                        item={item}
                        key={item.chunk_id}
                        editChunk={showChunkUpdatingModal}
                        checked={selectedChunkIds.some(
                          (x) => x === item.chunk_id,
                        )}
                        handleCheckboxClick={handleSingleCheckboxClick}
                        switchChunk={handleSwitchChunk}
                        clickChunkCard={handleChunkCardClick}
                        selected={item.chunk_id === selectedChunkId}
                        textMode={textMode}
                      ></ChunkCard>
                    ))}
                  </div>
                </div>
                <div className={styles.pageFooter}>
                  <RAGFlowPagination
                    pageSize={pagination.pageSize}
                    current={pagination.current}
                    total={total}
                    onChange={(page, pageSize) => {
                      onPaginationChange(page, pageSize);
                    }}
                  ></RAGFlowPagination>
                </div>
              </div>
            </Spin>
          </div>
        </div>
      </div>
      {chunkUpdatingVisible && (
        <CreatingModal
          doc_id={documentId}
          chunkId={chunkId}
          hideModal={hideChunkUpdatingModal}
          visible={chunkUpdatingVisible}
          loading={chunkUpdatingLoading}
          onOk={onChunkUpdatingOk}
          parserId={documentInfo.parser_id}
        />
      )}
    </>
  );
};

export default Chunk;
