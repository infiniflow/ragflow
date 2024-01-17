import React, { useEffect, useState } from 'react';
import { useNavigate, connect, Dispatch } from 'umi'
import { Button, Form, Input, InputNumber, Radio, Select, Tag, Space, Avatar, Divider, List, Skeleton } from 'antd';
import type { kSModelState } from './model'
import type { settingModelState } from '@/pages/setting/model'
import styles from './index.less'
const { CheckableTag } = Tag;
const layout = {
    labelCol: { span: 8 },
    wrapperCol: { span: 16 },
    labelAlign: 'left' as const
};
const { Option } = Select
/* eslint-disable no-template-curly-in-string */

interface kSProps {
    dispatch: Dispatch;
    kSModel: kSModelState;
    settingModel: settingModelState;
    id: string
}
const Index: React.FC<kSProps> = ({ settingModel, kSModel, dispatch, id }) => {
    let navigate = useNavigate();
    const { tenantIfo = {} } = settingModel
    const { parser_ids = '', embd_id = '' } = tenantIfo
    const [form] = Form.useForm();

    useEffect(() => {
        dispatch({
            type: 'settingModel/getTenantInfo',
            payload: {
            }
        });
        if (id) {

            dispatch({
                type: 'kSModel/getKbDetail',
                payload: {
                    kb_id: id
                },
                callback(detail: any) {
                    console.log(detail)
                    const { description, name, permission, embd_id } = detail
                    form.setFieldsValue({ description, name, permission, embd_id })
                    setSelectedTag(detail.parser_id)
                }
            });
        }

    }, [id])
    const [selectedTag, setSelectedTag] = useState('')
    const values = Form.useWatch([], form);
    console.log(values, '......变化')
    const onFinish = () => {
        form.validateFields().then(
            () => {
                if (id) {
                    dispatch({
                        type: 'kSModel/updateKb',
                        payload: {
                            ...values,
                            parser_id: selectedTag,
                            kb_id: id,
                            embd_id: undefined
                        }
                    });
                } else {
                    dispatch({
                        type: 'kSModel/createKb',
                        payload: {
                            ...values,
                            parser_id: selectedTag
                        },
                        callback(id: string) {
                            navigate(`/knowledge/add/setting?activeKey=file&id=${id}`);
                        }
                    });
                }
            },
            () => {

            },
        );



    };

    const handleChange = (tag: string, checked: boolean) => {
        const nextSelectedTag = checked
            ? tag
            : selectedTag;
        console.log('You are interested in: ', nextSelectedTag);
        setSelectedTag(nextSelectedTag);
    };

    return <Form
        {...layout}
        form={form}
        name="validateOnly"
        style={{ maxWidth: 1000, padding: 14 }}
    >
        <Form.Item name='name' label="知识库名称" rules={[{ required: true }]}>
            <Input />
        </Form.Item>
        <Form.Item name='description' label="知识库描述">
            <Input.TextArea />
        </Form.Item>
        <Form.Item name="permission" label="可见权限">
            <Radio.Group>
                <Radio value="me">只有我</Radio>
                <Radio value="team">所有团队成员</Radio>
            </Radio.Group>
        </Form.Item>
        <Form.Item
            name="embd_id"
            label="Embedding 模型"
            hasFeedback
            rules={[{ required: true, message: 'Please select your country!' }]}
        >
            <Select placeholder="Please select a country" >
                {embd_id.split(',').map((item: string) => {
                    return <Option value={item} key={item}>{item}</Option>
                })}

            </Select>
        </Form.Item>
        <div style={{ marginTop: '5px' }}>
            修改Embedding 模型，请去<span style={{ color: '#1677ff' }}>设置</span>
        </div>
        <Space size={[0, 8]} wrap>
            <div className={styles.tags}>
                {
                    parser_ids.split(',').map((tag: string) => {
                        return (<CheckableTag
                            key={tag}
                            checked={selectedTag === tag}
                            onChange={(checked) => handleChange(tag, checked)}
                        >
                            {tag}
                        </CheckableTag>)
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
            <Button type="primary" onClick={onFinish}>
                保存并处理
            </Button>
            <Button htmlType="button" style={{ marginLeft: '20px' }}>
                取消
            </Button>
        </Form.Item>
    </Form>
}



export default connect(({ settingModel, kSModel, loading }) => ({ settingModel, kSModel, loading }))(Index);