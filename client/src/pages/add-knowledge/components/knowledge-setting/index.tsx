import React, { useEffect, useState } from 'react';
import { Button, Form, Input, InputNumber, Radio, Select, Tag, Space, Avatar, Divider, List, Skeleton } from 'antd';
import InfiniteScroll from 'react-infinite-scroll-component';
import styles from './index.less'

const layout = {
    labelCol: { span: 8 },
    wrapperCol: { span: 16 },
    labelAlign: 'left' as const
};
const { Option } = Select
/* eslint-disable no-template-curly-in-string */
const validateMessages = {
    required: '${label} is required!',
    types: {
        email: '${label} is not a valid email!',
        number: '${label} is not a valid number!',
    },
    number: {
        range: '${label} must be between ${min} and ${max}',
    },
};
/* eslint-enable no-template-curly-in-string */

const onFinish = (values: any) => {
    console.log(values);
};
interface DataType {
    gender: string;
    name: {
        title: string;
        first: string;
        last: string;
    };
    email: string;
    picture: {
        large: string;
        medium: string;
        thumbnail: string;
    };
    nat: string;
}
const tags = [{ title: '研报' }, { title: '法律' }, { title: '简历' }, { title: '说明书' }, { title: '书籍' }, { title: '演讲稿' }]

const App: React.FC = () => {

    return <Form
        {...layout}
        name="nest-messages"
        onFinish={onFinish}
        style={{ maxWidth: 1000, padding: 14 }}
        validateMessages={validateMessages}
    >
        <Form.Item name={['user', 'name']} label="知识库名称" rules={[{ required: true }]}>
            <Input />
        </Form.Item>
        <Form.Item name={['user', 'introduction']} label="知识库描述">
            <Input.TextArea />
        </Form.Item>
        <Form.Item name="radio-group" label="可见权限">
            <Radio.Group>
                <Radio value="a">只有我</Radio>
                <Radio value="b">所有团队成员</Radio>
            </Radio.Group>
        </Form.Item>
        <Form.Item
            name="select"
            label="Embedding 模型"
            hasFeedback
            rules={[{ required: true, message: 'Please select your country!' }]}
        >
            <Select placeholder="Please select a country">
                <Option value="china">China</Option>
                <Option value="usa">U.S.A</Option>
            </Select>
            <div style={{ marginTop: '5px' }}>
                修改Embedding 模型，请去<span style={{ color: '#1677ff' }}>设置</span>
            </div>
        </Form.Item>
        <Space size={[0, 8]} wrap>
            <div className={styles.tags}>
                {
                    tags.map(item => {
                        return (<Tag key={item.title}>{item.title}</Tag>)
                    })
                }
            </div>
        </Space>
        <Space size={[0, 8]} wrap>

        </Space>
        <div className={styles.preset}>
            <div className={styles.left}>
                xxxxx文章
            </div>
            <div className={styles.right}>
                预估份数
            </div>
        </div>
        <Form.Item wrapperCol={{ ...layout.wrapperCol, offset: 8 }}>
            <Button type="primary" htmlType="submit">
                保存并处理
            </Button>
            <Button htmlType="button" style={{ marginLeft: '20px' }}>
                取消
            </Button>
        </Form.Item>
    </Form>
}



export default App;