import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { connect, Dispatch, useNavigate } from 'umi'
import { Space, Table, Input, Button, Switch, Dropdown, } from 'antd';
import type { MenuProps } from 'antd';
import { DownOutlined } from '@ant-design/icons'
import { debounce } from 'lodash';
import type { ColumnsType } from 'antd/es/table';
import UploadFile from './upload'
import CreateEPModal from './createEFileModal'
import SegmentSetModal from './segmentSetModal'
import styles from './index.less'
import type { kFModelState } from './model'

interface DataType {
    name: string;
    chunk_num: string;
    token_num: number;
    update_date: string;
    size: string;
    status: string;
    id: string;
    parser_id: string
}

interface kFProps {
    dispatch: Dispatch;
    kFModel: kFModelState;
    kb_id: string
}

const Index: React.FC<kFProps> = ({ kFModel, dispatch, kb_id }) => {
    const { data, loading } = kFModel
    const [inputValue, setInputValue] = useState('')
    const [doc_id, setDocId] = useState('0')
    const [parser_id, setParserId] = useState('0')
    let navigate = useNavigate();
    const getKfList = (keywords?: string) => {
        const payload = {
            kb_id,
            keywords
        }
        if (!keywords) {
            delete payload.keywords
        }
        dispatch({
            type: 'kFModel/getKfList',
            payload
        });
    }
    useEffect(() => {
        if (kb_id) {
            getKfList()
        }
    }, [kb_id])
    const debounceChange = debounce(getKfList, 300)
    const debounceCallback = useCallback((value: string) => debounceChange(value), [])
    const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
        const value = e.target.value
        setInputValue(value)
        debounceCallback(e.target.value)

    }
    const onChangeStatus = (e: boolean, doc_id: string) => {
        dispatch({
            type: 'kFModel/updateDocumentStatus',
            payload: {
                doc_id,
                status: Number(e)
            },
            callback() {
                getKfList()
            }
        });
    }
    const onRmDocument = () => {
        dispatch({
            type: 'kFModel/document_rm',
            payload: {
                doc_id
            },
            callback() {
                getKfList()
            }
        });

    }
    const showCEFModal = () => {
        dispatch({
            type: 'kFModel/updateState',
            payload: {
                isShowCEFwModal: true
            }
        });
    };

    const showSegmentSetModal = () => {
        dispatch({
            type: 'kFModel/updateState',
            payload: {
                isShowSegmentSetModal: true
            }
        });
    };
    const actionItems: MenuProps['items'] = useMemo(() => {
        return [
            {
                key: '1',
                label: (
                    <div>
                        <UploadFile kb_id={kb_id} getKfList={getKfList} />
                    </div>

                ),
            },
            {
                key: '2',
                label: (
                    <div>
                        <Button type="link" onClick={showCEFModal}> 导入虚拟文件</Button>
                    </div>
                ),
                // disabled: true,
            },
        ]
    }, [kb_id]);
    const chunkItems: MenuProps['items'] = [
        {
            key: '1',
            label: (
                <div>

                    <Button type="link" onClick={showSegmentSetModal}> 分段设置</Button>
                </div>

            ),
        },
        {
            key: '2',
            label: (
                <div>
                    <Button type="link" onClick={onRmDocument}> 删除</Button>
                </div>
            ),
            // disabled: true,
        },
    ]
    const toChunk = (id: string) => {
        console.log(id)
        navigate(`/knowledge/add/setting?activeKey=file&id=${kb_id}&doc_id=${id}`);
    }
    const columns: ColumnsType<DataType> = [
        {
            title: '名称',
            dataIndex: 'name',
            key: 'name',
            render: (text: any, { id }) => <div className={styles.tochunks} onClick={() => toChunk(id)}><img className={styles.img} src='https://gw.alipayobjects.com/zos/antfincdn/efFD%24IOql2/weixintupian_20170331104822.jpg' alt="" />{text}</div>,
            className: `${styles.column}`
        },
        {
            title: '数据总量',
            dataIndex: 'chunk_num',
            key: 'chunk_num',
            className: `${styles.column}`
        },
        {
            title: 'Tokens',
            dataIndex: 'token_num',
            key: 'token_num',
            className: `${styles.column}`
        },
        {
            title: '文件大小',
            dataIndex: 'size',
            key: 'size',
            className: `${styles.column}`
        },
        {
            title: '状态',
            key: 'status',
            dataIndex: 'status',
            className: `${styles.column}`,
            render: (_, { status: string, id }) => (
                <>
                    <Switch defaultChecked={status === '1'} onChange={(e) => {
                        onChangeStatus(e, id)
                    }} />
                </>
            ),
        },
        {
            title: 'Action',
            key: 'action',
            className: `${styles.column}`,
            render: (_, record) => (
                <Space size="middle">
                    <Dropdown menu={{ items: chunkItems }} trigger={['click']}>
                        <a onClick={() => {
                            setDocId(record.id)
                            setParserId(record.parser_id)
                        }}>
                            分段设置 <DownOutlined />
                        </a>
                    </Dropdown>
                </Space>
            ),
        },
    ];
    return <>
        <div className={styles.filter}>
            <div className="search">
                <Input placeholder="搜索" value={inputValue} style={{ width: 220 }} allowClear onChange={handleInputChange} />
            </div>
            <div className="operate">
                <Dropdown menu={{ items: actionItems }} trigger={['click']} >
                    <a>
                        导入文件 <DownOutlined />
                    </a>
                </Dropdown>

            </div>
        </div>
        <Table rowKey='id' columns={columns} dataSource={data} loading={loading} pagination={false} scroll={{ scrollToFirstRowOnChange: true, x: true }} />
        <CreateEPModal getKfList={getKfList} kb_id={kb_id} />
        <SegmentSetModal getKfList={getKfList} parser_id={parser_id} doc_id={doc_id} />
    </>
};

export default connect(({ kFModel, loading }) => ({ kFModel, loading }))(Index);