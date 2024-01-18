import React, { useEffect, useState, useCallback } from 'react';
import { useNavigate, connect, Dispatch } from 'umi'
import { Card, Row, Col, Input, Select, Switch, Pagination, Spin, Button, Popconfirm } from 'antd';
import { MinusSquareOutlined, DeleteOutlined, } from '@ant-design/icons';
import type { PaginationProps } from 'antd';
import { api_host } from '@/utils/api'
import CreateModal from './createModal'


import styles from './index.less'
import { debounce } from 'lodash';
import type { chunkModelState } from './model'
interface chunkProps {
  dispatch: Dispatch;
  chunkModel: chunkModelState;
  doc_id: string
}
const Index: React.FC<chunkProps> = ({ chunkModel, dispatch, doc_id }) => {
  const [keywords, SetKeywords] = useState('')
  const [available_int, setAvailableInt] = useState(-1)
  const navigate = useNavigate()
  const [pagination, setPagination] = useState({ page: 1, size: 30 })
  // const [datas, setDatas] = useState(data)
  const { data = [], total, loading } = chunkModel
  console.log(chunkModel)
  const getChunkList = (value?: string) => {
    dispatch({
      type: 'chunkModel/updateState',
      payload: {
        loading: true
      }
    });
    interface payloadType {
      doc_id: string;
      keywords?: string;
      available_int?: number
    }
    const payload: payloadType = {
      doc_id,
      keywords: value || keywords,
      available_int
    }
    if (payload.available_int === -1) {
      delete payload.available_int
    }
    dispatch({
      type: 'chunkModel/chunk_list',
      payload: {
        ...payload,
        ...pagination
      }
    });
  }
  const confirm = (id: string) => {
    console.log(id)
    dispatch({
      type: 'chunkModel/rm_chunk',
      payload: {
        chunk_ids: [id]
      },
      callback: getChunkList
    });
  };
  const handleEditchunk = (chunk_id?: string) => {
    dispatch({
      type: 'chunkModel/updateState',
      payload: {
        isShowCreateModal: true,
        chunk_id
      },
      callback: getChunkList
    });
  }
  const onShowSizeChange: PaginationProps['onShowSizeChange'] = (page, size) => {
    setPagination({ page, size })
  };
  const switchChunk = (id: string, available_int: boolean) => {
    dispatch({
      type: 'chunkModel/updateState',
      payload: {
        loading: true
      }
    });
    dispatch({
      type: 'chunkModel/switch_chunk',
      payload: {
        chunk_ids: [id],
        available_int: Number(available_int),
        doc_id
      },
      callback: getChunkList
    });
  }

  useEffect(() => {
    getChunkList()
  }, [doc_id, available_int, pagination])
  const debounceChange = debounce(getChunkList, 300)
  const debounceCallback = useCallback((value: string) => debounceChange(value), [])
  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
    const value = e.target.value
    SetKeywords(value)
    debounceCallback(value)
  }
  const handleSelectChange = (value: number) => {
    setAvailableInt(value)
  }
  console.log('loading', loading)
  return (<>
    <div className={styles.chunkPage}>
      <div className={styles.filter}>
        <div>
          <Input placeholder="搜索" style={{ width: 220 }} value={keywords} allowClear onChange={handleInputChange} />
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
        <Button onClick={() => { handleEditchunk() }} type='link'>添加分段</Button>
      </div>
      <div className={styles.pageContent}>
        <Spin spinning={loading} className={styles.spin} size='large'>
          <Row gutter={{ xs: 8, sm: 16, md: 24, lg: 24 }} >
            {
              data.map((item: any) => {
                return (<Col className="gutter-row" key={item.chunk_id} xs={24} sm={12} md={12} lg={8}>
                  <Card className={styles.card}
                    onClick={() => { handleEditchunk(item.chunk_id) }}
                  >
                    <img style={{ width: '50px' }} src={`${api_host}/document/image/${item.img_id}`} alt="" />
                    <div className={styles.container}>
                      <div className={styles.content}>
                        <span className={styles.context}>
                          {item.content_ltks}
                        </span>
                        <span className={styles.delete}>
                          <Switch size="small" defaultValue={item.available_int == '1'} onChange={(checked: boolean, e: any) => {
                            e.stopPropagation();
                            e.nativeEvent.stopImmediatePropagation(); switchChunk(item.chunk_id, checked)
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

    </div >
    <CreateModal doc_id={doc_id} getChunkList={getChunkList} />
  </>
  )
};

export default connect(({ chunkModel, loading }) => ({ chunkModel, loading }))(Index);