import React, { useEffect, useState, useCallback, } from 'react';
import { useNavigate, connect, Dispatch } from 'umi'
import { Card, Row, Col, Input, Select, Switch, Pagination, Spin, Button, Popconfirm } from 'antd';
import { MinusSquareOutlined, DeleteOutlined, } from '@ant-design/icons';
import type { PaginationProps } from 'antd';
import { api_host } from '@/utils/api'
import CreateModal from '../knowledge-chunk/components/createModal'


import styles from './index.less'
import { debounce } from 'lodash';
import type { kSearchModelState } from './model'
import type { chunkModelState } from '../knowledge-chunk/model'
interface chunkProps {
  dispatch: Dispatch;
  kSearchModel: kSearchModelState;
  chunkModel: chunkModelState;
  kb_id: string
}
const Index: React.FC<chunkProps> = ({ kSearchModel, chunkModel, dispatch, kb_id }) => {

  const { data = [], total, loading, d_list = [], question, doc_ids, pagination, } = kSearchModel
  const { chunk_id, doc_id, isShowCreateModal } = chunkModel
  const getChunkList = () => {
    dispatch({
      type: 'kSearchModel/updateState',
      payload: {
        loading: true
      }
    });
    interface payloadType {
      kb_id: string;
      question?: string;
      doc_ids: any[];
      similarity_threshold?: number
    }
    const payload: payloadType = {
      kb_id,
      question,
      doc_ids,
      similarity_threshold: 0.1
    }
    dispatch({
      type: 'kSearchModel/chunk_list',
      payload: {
        ...payload,
        ...pagination
      }
    });
  }
  const confirm = (id: string) => {
    console.log(id)
    dispatch({
      type: 'kSearchModel/rm_chunk',
      payload: {
        chunk_ids: [id]
      },
      callback: getChunkList
    });
  };
  const handleEditchunk = (item: any) => {
    const { chunk_id, doc_id } = item
    dispatch({
      type: 'chunkModel/updateState',
      payload: {
        isShowCreateModal: true,
        chunk_id,
        doc_id
      },
      callback: getChunkList
    });
  }
  const onShowSizeChange: PaginationProps['onShowSizeChange'] = (page, size) => {
    dispatch({
      type: 'kSearchModel/updateState',
      payload: {
        pagination: { page, size }
      }
    });
  };
  const switchChunk = (item: any, available_int: boolean) => {
    const { chunk_id, doc_id } = item
    dispatch({
      type: 'kSearchModel/updateState',
      payload: {
        loading: true
      }
    });
    dispatch({
      type: 'kSearchModel/switch_chunk',
      payload: {
        chunk_ids: [chunk_id],
        doc_id,
        available_int
      },
      callback: getChunkList
    });
  }

  useEffect(() => {
    if (kb_id) {
      dispatch({
        type: 'kSearchModel/getKfList',
        payload: {
          kb_id
        }
      });
    }
  }, [kb_id])

  useEffect(() => {
    getChunkList()
  }, [doc_ids, pagination, question])
  const debounceChange = debounce((value) => {
    dispatch({
      type: 'kSearchModel/updateState',
      payload: {
        question: value
      }
    });
  }, 300)
  const debounceCallback = useCallback((value: string) => debounceChange(value), [])
  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
    const value = e.target.value
    debounceCallback(value)
  }
  const handleSelectChange = (value:
    any[]) => {
    dispatch({
      type: 'kSearchModel/updateState',
      payload: {
        doc_ids: value
      }
    });
  }
  console.log('loading', loading)
  return (<>
    <div className={styles.chunkPage}>
      <div className={styles.filter}>
        <Select
          showSearch
          placeholder="文件列表"
          optionFilterProp="children"
          onChange={handleSelectChange}
          style={{ width: 300, marginBottom: 20 }}
          options={d_list}
          value={doc_ids}
          fieldNames={{ label: 'name', value: 'id' }}
          mode='multiple'
        />

        <Input.TextArea autoSize={{ minRows: 6, maxRows: 6 }} placeholder="搜索" style={{ width: 300 }} allowClear value={question} onChange={handleInputChange} />

      </div>
      <div className={styles.pageContainer}>
        <div className={styles.pageContent}>
          <Spin spinning={loading} className={styles.spin} size='large'>
            <Row gutter={{ xs: 8, sm: 16, md: 24, lg: 24 }} >
              {
                data.map((item: any) => {
                  return (<Col className="gutter-row" key={item.chunk_id} xs={24} sm={12} md={12} lg={8}>
                    <Card className={styles.card}
                      onClick={() => { handleEditchunk(item) }}
                    >
                      <img style={{ width: '50px' }} src={`${api_host}/document/image/${item.img_id}`} alt="" />
                      <div className={styles.container}>
                        <div className={styles.content}>
                          <span className={styles.context}>
                            {item.content_ltks}
                          </span>
                          <span className={styles.delete}>
                            <Switch size="small" defaultValue={item.doc_ids == '1'} onChange={(checked: boolean, e: any) => {
                              e.stopPropagation();
                              e.nativeEvent.stopImmediatePropagation(); switchChunk(item, checked)
                            }} />
                          </span>
                        </div>
                        <div className={styles.footer}>
                          <span className={styles.text}>
                            <MinusSquareOutlined />{item.doc_num}文档
                          </span>
                          <span className={styles.text}>
                            <MinusSquareOutlined />{item.chunk_num}个
                          </span>
                          <span className={styles.text}>
                            <MinusSquareOutlined />{item.token_num}千字符
                          </span>
                          <span style={{ float: 'right' }}>
                            <Popconfirm
                              title="Delete the task"
                              description="Are you sure to delete this task?"
                              onConfirm={(e: any) => {
                                e.stopPropagation();
                                e.nativeEvent.stopImmediatePropagation()
                                console.log(confirm)
                                confirm(item.chunk_id)

                              }}
                              okText="Yes"
                              cancelText="No"
                            >
                              <DeleteOutlined onClick={(e) => {
                                e.stopPropagation();
                                e.nativeEvent.stopImmediatePropagation()
                              }} />
                            </Popconfirm>

                          </span>
                        </div>

                      </div>
                    </Card>
                  </Col>)
                })
              }
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

    </div >
    <CreateModal getChunkList={getChunkList} isShowCreateModal={isShowCreateModal} chunk_id={chunk_id} doc_id={doc_id} />
  </>
  )
};

export default connect(({ kSearchModel, chunkModel, loading }) => ({ kSearchModel, chunkModel, loading }))(Index);