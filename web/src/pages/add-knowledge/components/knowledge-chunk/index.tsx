import { getOneNamespaceEffectsLoading } from '@/utils/storeUtil';
import type { PaginationProps } from 'antd';
import { Button, Input, Pagination, Select, Space, Spin } from 'antd';
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
  available_int?: number;
}

const Chunk = () => {
  const dispatch = useDispatch();
  const chunkModel: ChunkModelState = useSelector(
    (state: any) => state.chunkModel,
  );
  const [keywords, SetKeywords] = useState('');
  const [available_int, setAvailableInt] = useState(-1);
  const [searchParams] = useSearchParams();
  const [pagination, setPagination] = useState({ page: 1, size: 30 });
  const { data = [], total, chunk_id, isShowCreateModal } = chunkModel;
  const effects = useSelector((state: any) => state.loading.effects);
  const loading = getOneNamespaceEffectsLoading('chunkModel', effects, [
    'create_hunk',
    'chunk_list',
    'switch_chunk',
  ]);
  const documentId: string = searchParams.get('doc_id') || '';

  const getChunkList = (value?: string) => {
    const payload: PayloadType = {
      doc_id: documentId,
      keywords: value || keywords,
      available_int,
    };
    if (payload.available_int === -1) {
      delete payload.available_int;
    }
    dispatch({
      type: 'chunkModel/chunk_list',
      payload: {
        ...payload,
        ...pagination,
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

  const onShowSizeChange: PaginationProps['onShowSizeChange'] = (
    page,
    size,
  ) => {
    setPagination({ page, size });
  };

  const switchChunk = async (id: string, available_int: boolean) => {
    const retcode = await dispatch<any>({
      type: 'chunkModel/switch_chunk',
      payload: {
        chunk_ids: [id],
        available_int: Number(available_int),
        doc_id: documentId,
      },
    });

    retcode === 0 && getChunkList();
  };

  useEffect(() => {
    getChunkList();
  }, [documentId, available_int, pagination]);

  const debounceChange = debounce(getChunkList, 300);
  const debounceCallback = useCallback(
    (value: string) => debounceChange(value),
    [],
  );

  const handleInputChange = (
    e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>,
  ) => {
    const value = e.target.value;
    SetKeywords(value);
    debounceCallback(value);
  };
  const handleSelectChange = (value: number) => {
    setAvailableInt(value);
  };
  return (
    <>
      <div className={styles.chunkPage}>
        <ChunkToolBar></ChunkToolBar>
        <div className={styles.filter}>
          <div>
            <Input
              placeholder="搜索"
              style={{ width: 220 }}
              value={keywords}
              allowClear
              onChange={handleInputChange}
            />
            <Select
              showSearch
              placeholder="是否启用"
              optionFilterProp="children"
              value={available_int}
              onChange={handleSelectChange}
              style={{ width: 220 }}
              options={[
                {
                  value: -1,
                  label: '全部',
                },
                {
                  value: 1,
                  label: '启用',
                },
                {
                  value: 0,
                  label: '未启用',
                },
              ]}
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
                <ChunkCard item={item} key={item.chunk_id}></ChunkCard>
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
            onChange={onShowSizeChange}
            defaultPageSize={30}
            pageSizeOptions={[30, 60, 90]}
            defaultCurrent={pagination.page}
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
