import { api_host } from '@/utils/api';
import { getOneNamespaceEffectsLoading } from '@/utils/stroreUtil';
import { DeleteOutlined, MinusSquareOutlined } from '@ant-design/icons';
import type { PaginationProps } from 'antd';
import {
  Button,
  Card,
  Col,
  Input,
  Pagination,
  Popconfirm,
  Row,
  Select,
  Spin,
  Switch,
} from 'antd';
import { debounce } from 'lodash';
import React, { useCallback, useEffect, useState } from 'react';
import { useDispatch, useSearchParams, useSelector } from 'umi';
import CreateModal from './components/createModal';

import styles from './index.less';

interface PayloadType {
  doc_id: string;
  keywords?: string;
  available_int?: number;
}

const Chunk = () => {
  const dispatch = useDispatch();
  const chunkModel = useSelector((state: any) => state.chunkModel);
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
            <Row gutter={{ xs: 8, sm: 16, md: 24, lg: 24 }}>
              {data.map((item: any) => {
                return (
                  <Col
                    className="gutter-row"
                    key={item.chunk_id}
                    xs={24}
                    sm={12}
                    md={12}
                    lg={8}
                  >
                    <Card
                      className={styles.card}
                      onClick={() => {
                        handleEditchunk(item.chunk_id);
                      }}
                    >
                      <img
                        style={{ width: '50px' }}
                        src={`${api_host}/document/image/${item.img_id}`}
                        alt=""
                      />
                      <div className={styles.container}>
                        <div className={styles.content}>
                          <span className={styles.context}>
                            {item.content_ltks}
                          </span>
                          <span className={styles.delete}>
                            <Switch
                              size="small"
                              defaultValue={item.available_int == '1'}
                              onChange={(checked: boolean, e: any) => {
                                e.stopPropagation();
                                e.nativeEvent.stopImmediatePropagation();
                                switchChunk(item.chunk_id, checked);
                              }}
                            />
                          </span>
                        </div>
                        <div className={styles.footer}>
                          <span className={styles.text}>
                            <MinusSquareOutlined />
                            {item.doc_num}文档
                          </span>
                          <span className={styles.text}>
                            <MinusSquareOutlined />
                            {item.chunk_num}个
                          </span>
                          <span className={styles.text}>
                            <MinusSquareOutlined />
                            {item.token_num}千字符
                          </span>
                          <span style={{ float: 'right' }}>
                            <Popconfirm
                              title="Delete the task"
                              description="Are you sure to delete this task?"
                              onConfirm={(e: any) => {
                                e.stopPropagation();
                                e.nativeEvent.stopImmediatePropagation();
                                console.log(confirm);
                                confirm(item.chunk_id);
                              }}
                              okText="Yes"
                              cancelText="No"
                            >
                              <DeleteOutlined
                                onClick={(e) => {
                                  e.stopPropagation();
                                  e.nativeEvent.stopImmediatePropagation();
                                }}
                              />
                            </Popconfirm>
                          </span>
                        </div>
                      </div>
                    </Card>
                  </Col>
                );
              })}
            </Row>
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
