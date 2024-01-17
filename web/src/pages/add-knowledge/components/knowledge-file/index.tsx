import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { connect, Dispatch } from 'umi'
import { Space, Table, Tag, Input, Button, Switch, Popover, Dropdown, } from 'antd';
import type { MenuProps } from 'antd';
import { PlusOutlined, DownOutlined } from '@ant-design/icons'
import { debounce } from 'lodash';
import type { ColumnsType } from 'antd/es/table';
import UploadFile from './upload'
import CreateEPModal from './createEFileModal'
import SegmentSetModal from './segmentSetModal'
import styles from './index.less'
import type { kFModelState } from './model'
import type { settingModelState } from '@/pages/setting/model'

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
    settingModel: settingModelState;
    id: string
}

const Index: React.FC<kFProps> = ({ kFModel, dispatch, id }) => {
    const { data, loading } = kFModel
    const [inputValue, setInputValue] = useState('')
    const [doc_id, setDocId] = useState('0')
    const [parser_id, setParserId] = useState('0')
    const changeValue = (value: string) => {
        {
            console.log(value)
        }
    }
    const getKfList = () => {
        dispatch({
            type: 'kFModel/getKfList',
            payload: {
                kb_id: id
            }
        });
    }
    useEffect(() => {
        if (id) {
            getKfList()
        }
    }, [id])
    const debounceChange = debounce(changeValue, 300)
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
                        <UploadFile kb_id={id} getKfList={getKfList} />
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
    }, [id]);
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
    const columns: ColumnsType<DataType> = [
        {
            title: '名称',
            dataIndex: 'name',
            key: 'name',
            render: (text) => <a><img className={styles.img} src='https://gw.alipayobjects.com/zos/antfincdn/efFD%24IOql2/weixintupian_20170331104822.jpg' alt="" />{text}</a>,
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
                <Input placeholder="搜索" value={inputValue} allowClear onChange={handleInputChange} />
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
        <CreateEPModal getKfList={getKfList} kb_id={id} />
        <SegmentSetModal getKfList={getKfList} parser_id={parser_id} doc_id={doc_id} />
    </>
};

export default connect(({ kFModel, loading }) => ({ kFModel, loading }))(Index);