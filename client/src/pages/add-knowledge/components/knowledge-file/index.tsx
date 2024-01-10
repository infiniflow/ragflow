import React, { useCallback, useState } from 'react';
import { Space, Table, Tag, Input, Button } from 'antd';
import { PlusOutlined } from '@ant-design/icons'
import { debounce } from 'lodash';
import type { ColumnsType } from 'antd/es/table';
import styles from './idnex.less'

interface DataType {
    key: string;
    name: string;
    age: number;
    address: string;
    tags: string[];
}

const columns: ColumnsType<DataType> = [
    {
        title: 'Name',
        dataIndex: 'name',
        key: 'name',
        render: (text) => <a>{text}</a>,
    },
    {
        title: 'Age',
        dataIndex: 'age',
        key: 'age',
    },
    {
        title: 'Address',
        dataIndex: 'address',
        key: 'address',
    },
    {
        title: 'Tags',
        key: 'tags',
        dataIndex: 'tags',
        render: (_, { tags }) => (
            <>
                {tags.map((tag) => {
                    let color = tag.length > 5 ? 'geekblue' : 'green';
                    if (tag === 'loser') {
                        color = 'volcano';
                    }
                    return (
                        <Tag color={color} key={tag}>
                            {tag.toUpperCase()}
                        </Tag>
                    );
                })}
            </>
        ),
    },
    {
        title: 'Action',
        key: 'action',
        render: (_, record) => (
            <Space size="middle">
                <a>Invite {record.name}</a>
                <a>Delete</a>
            </Space>
        ),
    },
];

const data: DataType[] = [
    {
        key: '1',
        name: 'John Brown',
        age: 32,
        address: 'New York No. 1 Lake Park',
        tags: ['nice', 'developer'],
    },
    {
        key: '2',
        name: 'Jim Green',
        age: 42,
        address: 'London No. 1 Lake Park',
        tags: ['loser'],
    },
    {
        key: '3',
        name: 'Joe Black',
        age: 32,
        address: 'Sydney No. 1 Lake Park',
        tags: ['cool', 'teacher'],
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
    return <>
        <div className={styles.filter}>
            <div className="search">
                <Input placeholder="搜索" value={inputValue} allowClear onChange={handleInputChange} />
            </div>
            <div className="operate">
                <Button
                    type="primary"
                    icon={<PlusOutlined />}
                    onClick={() => { }}
                >
                    添加
                </Button>
            </div>
        </div>
        <Table columns={columns} dataSource={data} loading={loading} />
    </>
};

export default App;