import React, { useCallback, useState } from 'react';
import { Space, Table, Tag, Input, Button, Switch, Popover, Dropdown, } from 'antd';
import type { MenuProps } from 'antd';
import { PlusOutlined, DownOutlined } from '@ant-design/icons'
import { debounce } from 'lodash';
import type { ColumnsType } from 'antd/es/table';
import styles from './idnex.less'

interface DataType {
    key: string;
    name: string;
    age: number;
    address: string;
    status: boolean;
}
const onChangeStatus = () => { }


const data: DataType[] = [
    {
        key: '1',
        name: 'John Brown',
        age: 32,
        address: 'New York No. 1 Lake Park',
        status: true,
    },
    {
        key: '2',
        name: 'Jim Green',
        age: 42,
        address: 'London No. 1 Lake Park',
        status: true,
    },
    {
        key: '3',
        name: 'Joe Black',
        age: 32,
        address: 'Sydney No. 1 Lake Park',
        status: true,
    },
    {
        key: '4',
        name: 'John Brown',
        age: 32,
        address: 'New York No. 1 Lake Park',
        status: true,
    },
    {
        key: '5',
        name: 'Jim Green',
        age: 42,
        address: 'London No. 1 Lake Park',
        status: true,
    },
    {
        key: '6',
        name: 'Joe Black',
        age: 32,
        address: 'Sydney No. 1 Lake Park',
        status: true,
    }, {
        key: '7',
        name: 'John Brown',
        age: 32,
        address: 'New York No. 1 Lake Park',
        status: true,
    },
    {
        key: '8',
        name: 'Jim Green',
        age: 42,
        address: 'London No. 1 Lake Park',
        status: true,
    },
    {
        key: '9',
        name: 'Joe Black',
        age: 32,
        address: 'Sydney No. 1 Lake Park',
        status: true,
    },
];

const App: React.FC = () => {
    const [inputValue, setInputValue] = useState('')
    const [loading, setLoading] = useState(false)
    const changeValue = (value: string) => {
        {
            console.log(value)
            setLoading(false)
        }
    }
    const debounceChange = debounce(changeValue, 300)
    const debounceCallback = useCallback((value: string) => debounceChange(value), [])
    const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
        const value = e.target.value
        setLoading(true)
        setInputValue(value)
        debounceCallback(e.target.value)

    }
    const actionItems: MenuProps['items'] = [
        {
            key: '1',
            label: (
                <div>
                    <Button type="link">导入文件</Button>
                </div>

            ),
        },
        {
            key: '2',
            label: (
                <div>
                    <Button type="link"> 导入虚拟文件</Button>
                </div>
            ),
            // disabled: true,
        },
    ];

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
            dataIndex: 'total',
            key: 'total',
            className: `${styles.column}`
        },
        {
            title: 'Tokens',
            dataIndex: 'tokens',
            key: 'tokens',
            className: `${styles.column}`
        },
        {
            title: '状态',
            key: 'status',
            dataIndex: 'status',
            className: `${styles.column}`,
            render: (_, { status }) => (
                <>
                    <Switch defaultChecked onChange={onChangeStatus} />
                </>
            ),
        },
        {
            title: 'Action',
            key: 'action',
            className: `${styles.column}`,
            render: (_, record) => (
                <Space size="middle">
                    <Dropdown menu={{ items: actionItems }}>
                        <a>
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
        <Table columns={columns} dataSource={data} loading={loading} pagination={false} scroll={{ scrollToFirstRowOnChange: true, x: true }} />
    </>
};

export default App;