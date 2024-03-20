import { useFetchChunkList } from '@/hooks/chunkHooks';
import { useDeleteChunkByIds } from '@/hooks/knowledgeHook';
import type { PaginationProps } from 'antd';
import { Divider, Flex, Pagination, Space, Spin, message } from 'antd';
import classNames from 'classnames';
import { useCallback, useEffect, useState } from 'react';
import { useDispatch, useSearchParams, useSelector } from 'umi';
import ChunkCard from './components/chunk-card';
import CreatingModal from './components/chunk-creating-modal';
import ChunkToolBar from './components/chunk-toolbar';
import DocumentPreview from './components/document-preview/preview';
import {
  useHandleChunkCardClick,
  useSelectChunkListLoading,
  useSelectDocumentInfo,
} from './hooks';
import { ChunkModelState } from './model';

import styles from './index.less';

const Chunk = () => {
  const dispatch = useDispatch();
  const chunkModel: ChunkModelState = useSelector(
    (state: any) => state.chunkModel,
  );
  const [selectedChunkIds, setSelectedChunkIds] = useState<string[]>([]);
  const [searchParams] = useSearchParams();
  const { data = [], total, pagination } = chunkModel;
  const loading = useSelectChunkListLoading();
  const documentId: string = searchParams.get('doc_id') || '';
  const [chunkId, setChunkId] = useState<string | undefined>();
  const { removeChunk } = useDeleteChunkByIds();
  const documentInfo = useSelectDocumentInfo();
  const { handleChunkCardClick, selectedChunkId } = useHandleChunkCardClick();
  const isPdf = documentInfo.type === 'pdf';

  const getChunkList = useFetchChunkList();

  const handleEditChunk = useCallback(
    (chunk_id?: string) => {
      setChunkId(chunk_id);

      dispatch({
        type: 'chunkModel/setIsShowCreateModal',
        payload: true,
      });
    },
    [dispatch],
  );

  const onPaginationChange: PaginationProps['onShowSizeChange'] = (
    page,
    size,
  ) => {
    setSelectedChunkIds([]);
    dispatch({
      type: 'chunkModel/setPagination',
      payload: {
        current: page,
        pageSize: size,
      },
    });
    getChunkList();
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
  const showSelectedChunkWarning = () => {
    message.warning('Please select chunk!');
  };

  const handleRemoveChunk = useCallback(async () => {
    if (selectedChunkIds.length > 0) {
      const resCode: number = await removeChunk(selectedChunkIds, documentId);
      if (resCode === 0) {
        setSelectedChunkIds([]);
      }
    } else {
      showSelectedChunkWarning();
    }
  }, [selectedChunkIds, documentId, removeChunk]);

  const switchChunk = useCallback(
    async (available?: number, chunkIds?: string[]) => {
      let ids = chunkIds;
      if (!chunkIds) {
        ids = selectedChunkIds;
        if (selectedChunkIds.length === 0) {
          showSelectedChunkWarning();
          return;
        }
      }

      const resCode: number = await dispatch<any>({
        type: 'chunkModel/switch_chunk',
        payload: {
          chunk_ids: ids,
          available_int: available,
          doc_id: documentId,
        },
      });
      if (!chunkIds && resCode === 0) {
        getChunkList();
      }
    },
    [dispatch, documentId, getChunkList, selectedChunkIds],
  );

  useEffect(() => {
    getChunkList();
    return () => {
      dispatch({
        type: 'chunkModel/resetFilter', // TODO: need to reset state uniformly
      });
    };
  }, [dispatch, getChunkList]);

  return (
    <>
      <div className={styles.chunkPage}>
        <ChunkToolBar
          getChunkList={getChunkList}
          selectAllChunk={selectAllChunk}
          createChunk={handleEditChunk}
          removeChunk={handleRemoveChunk}
          checked={selectedChunkIds.length === data.length}
          switchChunk={switchChunk}
        ></ChunkToolBar>
        <Divider></Divider>
        <Flex flex={1} gap={'middle'}>
          <Flex
            vertical
            className={isPdf ? styles.pagePdfWrapper : styles.pageWrapper}
          >
            <Spin spinning={loading} className={styles.spin} size="large">
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
                      editChunk={handleEditChunk}
                      checked={selectedChunkIds.some(
                        (x) => x === item.chunk_id,
                      )}
                      handleCheckboxClick={handleSingleCheckboxClick}
                      switchChunk={switchChunk}
                      clickChunkCard={handleChunkCardClick}
                      selected={item.chunk_id === selectedChunkId}
                    ></ChunkCard>
                  ))}
                </Space>
              </div>
            </Spin>
            <div className={styles.pageFooter}>
              <Pagination
                responsive
                showLessItems
                showQuickJumper
                showSizeChanger
                onChange={onPaginationChange}
                pageSize={pagination.pageSize}
                pageSizeOptions={[10, 30, 60, 90]}
                current={pagination.current}
                size={'small'}
                total={total}
              />
            </div>
          </Flex>

          {isPdf && (
            <section className={styles.documentPreview}>
              <DocumentPreview
                selectedChunkId={selectedChunkId}
              ></DocumentPreview>
            </section>
          )}
        </Flex>
      </div>
      <CreatingModal doc_id={documentId} chunkId={chunkId} />
    </>
  );
};

export default Chunk;
