import React, { useCallback, useEffect, useState } from 'react';
import { connect, useNavigate, useLocation } from 'umi'
import { Space, Table, Tag, Input, Button, Switch, Popover, Dropdown, } from 'antd';
import type { MenuProps } from 'antd';
import { PlusOutlined, DownOutlined } from '@ant-design/icons'
import { debounce } from 'lodash';
import type { ColumnsType } from 'antd/es/table';
import Upload from './upload'
import CreateEPModal from './createEFileModal'
import styles from './idnex.less'

interface DataType {
    name: string;
    chunk_num: string;
    token_num: number;
    update_date: string;
    size: string;
    status: boolean;
    id: string
}



const Index: React.FC = ({ kFModel, dispatch, id }) => {
    const { data, loading } = kFModel
    const [inputValue, setInputValue] = useState('')
    const [doc_id, setDocId] = useState('0')
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
    const onChangeStatus = (e, doc_id) => {
        console.log(doc_id)
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
    const onRmDocument = (doc_id) => {
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

    const actionItems: MenuProps['items'] = [
        {
            key: '1',
            label: (
                <div>
                    <Upload kb_id={id} getKfList={getKfList} />
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
    ];
    const getItems = useCallback((id) => {
        console.log(id)
        return [
            {
                key: '1',
                label: (
                    <div>

                        <Button type="link"> 分段设置</Button>
                    </div>

                ),
            },
            {
                key: '2',
                label: (
                    <div>
                        <Button type="link" onClick={() => onRmDocument(id)}> 删除</Button>
                    </div>
                ),
                // disabled: true,
            },
        ];
    }, [])
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
            render: (_, { status, id }) => (
                <>
                    <Switch defaultChecked={status == 1} onChange={(e) => {
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
                    <Dropdown menu={{ items: getItems(record.id) }}>
                        <a onClick={() => { setDocId(record.id) }}>
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
                <Dropdown menu={{ items: actionItems }}>
                    <a>
                        导入文件 <DownOutlined />
                    </a>
                </Dropdown>

            </div>
        </div>
        <Table rowKey='id' columns={columns} dataSource={data} loading={loading} pagination={false} scroll={{ scrollToFirstRowOnChange: true, x: true }} />
        <CreateEPModal getKfList={getKfList} kb_id={id} />
    </>
};

export default connect(({ kFModel, loading }) => ({ kFModel, loading }))(Index);