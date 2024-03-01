import { getOneNamespaceEffectsLoading } from '@/utils/storeUtil';
import type { PaginationProps } from 'antd';
import { Divider, Flex, Pagination, Space, Spin, message } from 'antd';
import { useCallback, useEffect, useState } from 'react';
import { useDispatch, useSearchParams, useSelector } from 'umi';
import CreatingModal from './components/chunk-creating-modal';

import { useDeleteChunkByIds } from '@/hooks/knowledgeHook';
import ChunkCard from './components/chunk-card';
import ChunkToolBar from './components/chunk-toolbar';
import DocumentPreview from './components/document-preview';
import { useHandleChunkCardClick, useSelectDocumentInfo } from './hooks';
import styles from './index.less';
import { ChunkModelState } from './model';

interface PayloadType {
  doc_id: string;
  keywords?: string;
}

const Chunk = () => {
  const dispatch = useDispatch();
  const chunkModel: ChunkModelState = useSelector(
    (state: any) => state.chunkModel,
  );
  const [selectedChunkIds, setSelectedChunkIds] = useState<string[]>([]);
  const [searchParams] = useSearchParams();
  const { data = [], total, pagination } = chunkModel;
  const effects = useSelector((state: any) => state.loading.effects);
  const loading = getOneNamespaceEffectsLoading('chunkModel', effects, [
    'create_hunk',
    'chunk_list',
    'switch_chunk',
  ]);
  const documentId: string = searchParams.get('doc_id') || '';
  const [chunkId, setChunkId] = useState<string | undefined>();
  const { removeChunk } = useDeleteChunkByIds();
  const documentInfo = useSelectDocumentInfo();
  const { handleChunkCardClick, selectedChunkId } = useHandleChunkCardClick();

  const getChunkList = useCallback(() => {
    const payload: PayloadType = {
      doc_id: documentId,
    };

    dispatch({
      type: 'chunkModel/chunk_list',
      payload: {
        ...payload,
      },
    });
  }, [dispatch, documentId]);

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
          <Flex flex={1} vertical>
            <div className={styles.pageContent}>
              <Spin spinning={loading} className={styles.spin} size="large">
                <Space
                  direction="vertical"
                  size={'middle'}
                  className={styles.chunkContainer}
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
              </Spin>
            </div>
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
                total={total}
              />
            </div>
          </Flex>

          {documentInfo.type === 'pdf' && (
            <section className={styles.documentPreview}>
              <DocumentPreview></DocumentPreview>
            </section>
          )}
        </Flex>
      </div>
      <CreatingModal doc_id={documentId} chunkId={chunkId} />
    </>
  );
};

export default Chunk;
