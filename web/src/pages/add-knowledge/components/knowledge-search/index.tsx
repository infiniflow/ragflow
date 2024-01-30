import { api_host } from '@/utils/api';
import { DeleteOutlined, MinusSquareOutlined } from '@ant-design/icons';
import type { PaginationProps } from 'antd';
import {
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
import React, { useCallback, useEffect } from 'react';
import { useDispatch, useSelector } from 'umi';
import CreateModal from '../knowledge-chunk/components/createModal';

import { useOneNamespaceEffectsLoading } from '@/hooks/storeHooks';
import { debounce } from 'lodash';
import styles from './index.less';
interface chunkProps {
  kb_id: string;
}

const KnowledgeSearching: React.FC<chunkProps> = ({ kb_id }) => {
  const dispatch = useDispatch();
  const kSearchModel = useSelector((state: any) => state.kSearchModel);
  const chunkModel = useSelector((state: any) => state.chunkModel);
  const loading = useOneNamespaceEffectsLoading('kSearchModel', [
    'chunk_list',
    'switch_chunk',
  ]);

  const {
    data = [],
    total,
    d_list = [],
    question,
    doc_ids,
    pagination,
  } = kSearchModel;
  const { chunk_id, doc_id, isShowCreateModal } = chunkModel;

  const getChunkList = () => {
    dispatch({
      type: 'kSearchModel/chunk_list',
      payload: {
        kb_id,
      },
    });
  };
  const confirm = (id: string) => {
    dispatch({
      type: 'kSearchModel/rm_chunk',
      payload: {
        chunk_ids: [id],
        kb_id,
      },
    });
  };
  const handleEditchunk = (item: any) => {
    const { chunk_id, doc_id } = item;
    dispatch({
      type: 'chunkModel/updateState',
      payload: {
        isShowCreateModal: true,
        chunk_id,
        doc_id,
      },
    });
    getChunkList();
  };
  const onShowSizeChange: PaginationProps['onShowSizeChange'] = (
    page,
    size,
  ) => {
    dispatch({
      type: 'kSearchModel/updateState',
      payload: {
        pagination: { page, size },
      },
    });
  };
  useEffect(() => {
    dispatch({
      type: 'kSearchModel/updateState',
      payload: {
        doc_ids: [],
        question: '',
      },
    });
    dispatch({
      type: 'kSearchModel/getKfList',
      payload: {
        kb_id,
      },
    });
  }, []);
  const switchChunk = (item: any, available_int: boolean) => {
    const { chunk_id, doc_id } = item;

    dispatch({
      type: 'kSearchModel/switch_chunk',
      payload: {
        chunk_ids: [chunk_id],
        doc_id,
        available_int,
        kb_id,
      },
    });
  };

  useEffect(() => {
    getChunkList();
  }, [doc_ids, pagination, question]);
  const debounceChange = debounce((value) => {
    dispatch({
      type: 'kSearchModel/updateState',
      payload: {
        question: value,
      },
    });
  }, 300);

  const debounceCallback = useCallback(
    (value: string) => debounceChange(value),
    [],
  );
  const handleInputChange = (
    e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>,
  ) => {
    const value = e.target.value;
    debounceCallback(value);
  };
  const handleSelectChange = (value: any[]) => {
    dispatch({
      type: 'kSearchModel/updateState',
      payload: {
        doc_ids: value,
      },
    });
  };

  return (
    <>
      <div className={styles.chunkPage}>
        <div className={styles.filter}>
          <Select
            showSearch
            placeholder="文件列表"
            optionFilterProp="children"
            onChange={handleSelectChange}
            style={{ width: 300, marginBottom: 20 }}
            options={d_list}
            fieldNames={{ label: 'name', value: 'id' }}
            mode="multiple"
          />

          <Input.TextArea
            autoSize={{ minRows: 6, maxRows: 6 }}
            placeholder="搜索"
            style={{ width: 300 }}
            allowClear
            onChange={handleInputChange}
          />
        </div>
        <div className={styles.pageContainer}>
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
                          handleEditchunk(item);
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
                                defaultValue={item.doc_ids == '1'}
                                onChange={(checked: boolean, e: any) => {
                                  e.stopPropagation();
                                  e.nativeEvent.stopImmediatePropagation();
                                  switchChunk(item, checked);
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
      </div>
      <CreateModal
        getChunkList={getChunkList}
        isShowCreateModal={isShowCreateModal}
        chunk_id={chunk_id}
        doc_id={doc_id}
      />
    </>
  );
};

export default KnowledgeSearching;
