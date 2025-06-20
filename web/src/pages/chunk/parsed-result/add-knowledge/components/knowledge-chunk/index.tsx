import { useFetchNextChunkList, useSwitchChunk } from '@/hooks/chunk-hooks';
import type { PaginationProps } from 'antd';
import { Flex, Pagination, Space, Spin, message } from 'antd';
import classNames from 'classnames';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import ChunkCard from './components/chunk-card';
import CreatingModal from './components/chunk-creating-modal';
import DocumentPreview from './components/document-preview/preview';
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

  const { t } = useTranslation();
  const { changeChunkTextMode, textMode } = useChangeChunkTextMode();
  const { switchChunk } = useSwitchChunk();
  const {
    chunkUpdatingLoading,
    onChunkUpdatingOk,
    showChunkUpdatingModal,
    hideChunkUpdatingModal,
    chunkId,
    chunkUpdatingVisible,
    documentId,
  } = useUpdateChunk();

  const onPaginationChange: PaginationProps['onShowSizeChange'] = (
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
      if (!chunkIds && resCode === 0) {
      }
    },
    [switchChunk, documentId, selectedChunkIds, showSelectedChunkWarning],
  );

  const { highlights, setWidthAndHeight } =
    useGetChunkHighlights(selectedChunkId);

  return (
    <>
      <div className={styles.chunkPage}>
        {/* <ChunkToolBar
          selectAllChunk={selectAllChunk}
          createChunk={showChunkUpdatingModal}
          removeChunk={handleRemoveChunk}
          checked={selectedChunkIds.length === data.length}
          switchChunk={handleSwitchChunk}
          changeChunkTextMode={changeChunkTextMode}
          searchString={searchString}
          handleInputChange={handleInputChange}
          available={available}
          handleSetAvailable={handleSetAvailable}
        ></ChunkToolBar> */}
        {/* <Divider></Divider> */}
        <Flex flex={1} gap={'middle'}>
          <div className="w-[40%]">
            <div className="h-[100px] flex flex-col justify-end pb-[5px]">
              <DocumentHeader {...documentInfo} />
            </div>
            {isPdf && (
              <section className={styles.documentPreview}>
                <DocumentPreview
                  highlights={highlights}
                  setWidthAndHeight={setWidthAndHeight}
                ></DocumentPreview>
              </section>
            )}
          </div>
          <Flex
            vertical
            className={isPdf ? styles.pagePdfWrapper : styles.pageWrapper}
          >
            <Spin spinning={loading} className={styles.spin} size="large">
              <div className="h-[100px] flex flex-col justify-end pb-[5px]">
                <div>
                  <h2 className="text-[24px]">Chunk Result</h2>
                  <div className="text-[14px] text-[#979AAB]">
                    View the chunked segments used for embedding and retrieval.
                  </div>
                </div>
              </div>
              <div className=" rounded-[16px] bg-[#FFF]/10 pl-[20px] pb-[20px] pt-[20px] box-border	">
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
                    checked={selectedChunkIds.length === data.length}
                  />
                </div>
                <div className={styles.pageContent}>
                  <Space
                    direction="vertical"
                    size={'middle'}
                    className={classNames(styles.chunkContainer, {
                      [styles.chunkOtherContainer]: !isPdf,
                    })}
                  >
                    {data.map((item) => (
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
                  </Space>
                </div>
              </div>
            </Spin>
            <div className={styles.pageFooter}>
              <Pagination
                {...pagination}
                total={total}
                size={'small'}
                onChange={onPaginationChange}
              />
            </div>
          </Flex>
        </Flex>
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
