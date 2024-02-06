import { getOneNamespaceEffectsLoading } from '@/utils/storeUtil';
import type { PaginationProps } from 'antd';
import { Button, Input, Pagination, Space, Spin } from 'antd';
import { debounce } from 'lodash';
import React, { useCallback, useEffect, useState } from 'react';
import { useDispatch, useSearchParams, useSelector } from 'umi';
import CreateModal from './components/createModal';

import ChunkCard from './components/chunk-card';
import ChunkToolBar from './components/chunk-toolbar';
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
  const [keywords, SetKeywords] = useState('');
  const [selectedChunkIds, setSelectedChunkIds] = useState<string[]>([]);
  const [searchParams] = useSearchParams();
  const {
    data = [],
    total,
    chunk_id,
    isShowCreateModal,
    pagination,
  } = chunkModel;
  const effects = useSelector((state: any) => state.loading.effects);
  const loading = getOneNamespaceEffectsLoading('chunkModel', effects, [
    'create_hunk',
    'chunk_list',
    'switch_chunk',
  ]);
  const documentId: string = searchParams.get('doc_id') || '';

  const getChunkList = () => {
    const payload: PayloadType = {
      doc_id: documentId,
    };

    dispatch({
      type: 'chunkModel/chunk_list',
      payload: {
        ...payload,
      },
    });
  };

  const confirm = async (id: string) => {
    const retcode = await dispatch<any>({
      type: 'chunkModel/rm_chunk',
      payload: {
        chunk_ids: [id],
      },
    });

    retcode === 0 && getChunkList();
  };

  const handleEditchunk = (chunk_id?: string) => {
    dispatch({
      type: 'chunkModel/updateState',
      payload: {
        isShowCreateModal: true,
        chunk_id,
        doc_id: documentId,
      },
    });
    getChunkList();
  };

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
      // setSelectedChunkIds((previousIds) => {
      //   return checked ? [...previousIds, ...data.map((x) => x.chunk_id)] : [];
      // });
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

  useEffect(() => {
    getChunkList();
    return () => {
      dispatch({
        type: 'chunkModel/resetFilter', // TODO: need to reset state uniformly
      });
    };
  }, [documentId]);

  const debounceChange = debounce(getChunkList, 300);
  const debounceCallback = useCallback(
    (value: string) => debounceChange(value),
    [],
  );

  const handleInputChange = (
    e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>,
  ) => {
    setSelectedChunkIds([]);
    const value = e.target.value;
    SetKeywords(value);
    debounceCallback(value);
  };

  return (
    <>
      <div className={styles.chunkPage}>
        <ChunkToolBar
          getChunkList={getChunkList}
          selectAllChunk={selectAllChunk}
          checked={selectedChunkIds.length === data.length}
        ></ChunkToolBar>
        <div className={styles.filter}>
          <div>
            <Input
              placeholder="搜索"
              style={{ width: 220 }}
              value={keywords}
              allowClear
              onChange={handleInputChange}
            />
          </div>
          <Button
            onClick={() => {
              handleEditchunk();
            }}
            type="link"
          >
            添加分段
          </Button>
        </div>
        <div className={styles.pageContent}>
          <Spin spinning={loading} className={styles.spin} size="large">
            <Space direction="vertical" size={'middle'}>
              {data.map((item) => (
                <ChunkCard
                  item={item}
                  key={item.chunk_id}
                  checked={selectedChunkIds.some((x) => x === item.chunk_id)}
                  handleCheckboxClick={handleSingleCheckboxClick}
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
            defaultPageSize={10}
            pageSizeOptions={[10, 30, 60, 90]}
            defaultCurrent={pagination.current}
            total={total}
          />
        </div>
      </div>
      <CreateModal
        doc_id={documentId}
        isShowCreateModal={isShowCreateModal}
        chunk_id={chunk_id}
        getChunkList={getChunkList}
      />
    </>
  );
};

export default Chunk;
