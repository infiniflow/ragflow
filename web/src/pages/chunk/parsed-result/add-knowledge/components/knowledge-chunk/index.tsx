import {
  useFetchNextChunkList,
  useSwitchChunk,
} from '@/hooks/use-chunk-request';
import classNames from 'classnames';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import ChunkCard from './components/chunk-card';
import CreatingModal from './components/chunk-creating-modal';
import {
  useChangeChunkTextMode,
  useDeleteChunkByIds,
  useGetChunkHighlights,
  useHandleChunkCardClick,
  useUpdateChunk,
} from './hooks';

import ChunkResultBar from './components/chunk-result-bar';
import CheckboxSets from './components/chunk-result-bar/checkbox-sets';
// import DocumentHeader from './components/document-preview/document-header';

import DocumentPreview from '@/components/document-preview';
import DocumentHeader from '@/components/document-preview/document-header';
import { useGetDocumentUrl } from '@/components/document-preview/hooks';
import { PageHeader } from '@/components/page-header';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
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
import { LucideArrowBigLeft } from 'lucide-react';
import styles from './index.module.less';

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
    dataUpdatedAt,
  } = useFetchNextChunkList();
  const { handleChunkCardClick, selectedChunkId } = useHandleChunkCardClick();
  const isPdf = documentInfo?.type === 'pdf';

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
  const { navigateToDataFile, getQueryString } = useNavigatePage();
  const fileUrl = useGetDocumentUrl(false);
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
        return documentInfo?.name.split('.').pop() || 'visual';
      case 'docx':
      case 'txt':
      case 'md':
      case 'mdx':
      case 'pdf':
        return documentInfo?.type;
    }
    return 'unknown';
  }, [documentInfo]);

  return (
    <main className="h-dvh flex flex-col">
      <PageHeader>
        <Button
          variant="outline"
          onClick={navigateToDataFile(
            getQueryString(QueryStringMap.id) as string,
          )}
        >
          <LucideArrowBigLeft />
          {t('common.back')}
        </Button>
      </PageHeader>

      <Card className="mx-5 mb-5 flex-1 h-0 p-0 bg-transparent shadow-none">
        <CardContent className="p-0 h-full flex flex-row divide-x-0.5 rtl:divide-x-reverse">
          <article className="w-2/5 flex flex-col">
            <DocumentHeader className="flex-0 p-5 pb-0" {...documentInfo} />

            <div className="flex-1 h-0 min-h-0 overflow-hidden p-5 pt-2.5 [&>section]:h-full [&>section]:min-h-0">
              <DocumentPreview
                className="h-full min-h-0 overflow-auto [&_img]:max-w-full [&_img]:h-auto"
                fileType={fileType}
                highlights={highlights}
                setWidthAndHeight={setWidthAndHeight}
                url={fileUrl}
              />
            </div>
          </article>

          <article
            className={classNames(
              { [styles.pagePdfWrapper]: isPdf },
              'flex flex-col w-3/5',
            )}
          >
            <header className="flex-0 p-5 pb-2.5 border-b-0.5 border-b-border-button">
              <h2 className="text-[24px]">{t('chunk.chunkResult')}</h2>
              <div className="text-[14px] text-text-secondary">
                {t('chunk.chunkResultTip')}
              </div>
            </header>

            <Spin spinning={loading} className="flex-1 h-0" size="large">
              <div className="relative @container h-full px-5 pb-5 overflow-x-hidden overflow-y-auto">
                <div
                  className="
                    sticky top-0 z-[1] bg-bg-base space-y-4 py-5
                    @4xl:flex @4xl:justify-between @4xl:items-center
                    @4xl:space-y-0 @4xl:gap-4
                  "
                  role="toolbar"
                >
                  <ChunkResultBar
                    className="@4xl:order-2"
                    handleInputChange={handleInputChange}
                    searchString={searchString}
                    changeChunkTextMode={changeChunkTextMode}
                    createChunk={showChunkUpdatingModal}
                    available={available}
                    selectAllChunk={selectAllChunk}
                    handleSetAvailable={handleSetAvailable}
                  />

                  <CheckboxSets
                    className="h-8"
                    selectAllChunk={selectAllChunk}
                    switchChunk={handleSwitchChunk}
                    removeChunk={handleRemoveChunk}
                    checked={selectedChunkIds.length === data.length}
                    selectedChunkIds={selectedChunkIds}
                  />
                </div>

                <div className="space-y-4">
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
                      t={dataUpdatedAt}
                    />
                  ))}
                </div>

                <footer className="mt-5">
                  <RAGFlowPagination
                    pageSize={pagination.pageSize}
                    current={pagination.current}
                    total={total}
                    onChange={(page, pageSize) => {
                      onPaginationChange(page, pageSize);
                    }}
                  />
                </footer>
              </div>
            </Spin>
          </article>
        </CardContent>
      </Card>

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
    </main>
  );
};

export default Chunk;
