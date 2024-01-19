import React, { useEffect, useState } from 'react'
import { connect, Dispatch } from 'umi';
import i18n from 'i18next';
import { useTranslation, Trans } from 'react-i18next'
import { Input, Modal, Form } from 'antd'
import styles from './index.less';
import type { chunkModelState } from './model'
import EditTag from './editTag'

type FieldType = {
    content_ltks?: string;
};
interface kFProps {
    dispatch: Dispatch;
    chunkModel: chunkModelState;
    getChunkList: () => void;
    isShowCreateModal: boolean;
    doc_id: string;
    chunk_id: string
}
const Index: React.FC<kFProps> = ({ dispatch, getChunkList, doc_id, isShowCreateModal, chunk_id }) => {
    // const { , chunkInfo } = chunkModel
    const [important_kwd, setImportantKwd] = useState(['Unremovable', 'Tag 2', 'Tag 3']);
    const { t } = useTranslation()
    const handleCancel = () => {
        dispatch({
            type: 'chunkModel/updateState',
            payload: {
                isShowCreateModal: false
            }
        });
    };
    useEffect(() => {
        console.log(chunk_id, isShowCreateModal)
        if (chunk_id && isShowCreateModal) {
            dispatch({
                type: 'chunkModel/get_chunk',
                payload: {
                    chunk_id
                },
                callback(info: any) {
                    console.log(info)
                    const { content_ltks, important_kwd = [] } = info
                    form.setFieldsValue({ content_ltks })
                    setImportantKwd(important_kwd)
                }
            });
        }
    }, [chunk_id, isShowCreateModal])
    const [form] = Form.useForm()
    const handleOk = async () => {
        try {
            const values = await form.validateFields();
            dispatch({
                type: 'chunkModel/create_hunk',
                payload: {
                    content_ltks: values.content_ltks,
                    doc_id,
                    chunk_id,
                    important_kwd
                },
                callback: () => {
                    dispatch({
                        type: 'chunkModel/updateState',
                        payload: {
                            isShowCreateModal: false
                        }
                    });
                    getChunkList && getChunkList()
                }
            });

        } catch (errorInfo) {
            console.log('Failed:', errorInfo);
        }
    };

    return (
        <Modal title="Basic Modal" open={isShowCreateModal} onOk={handleOk} onCancel={handleCancel}>
            <Form
                form={form}
                name="validateOnly"
                labelCol={{ span: 5 }}
                wrapperCol={{ span: 19 }}
                style={{ maxWidth: 600 }}
                autoComplete="off"
            >
                <Form.Item<FieldType>
                    label="chunk 内容"
                    name="content_ltks"
                    rules={[{ required: true, message: 'Please input name!' }]}
                >
                    <Input.TextArea />
                </Form.Item>
                <EditTag tags={important_kwd} setTags={setImportantKwd} />
            </Form>
        </Modal >


    );
}
export default connect(({ chunkModel, loading }) => ({ chunkModel, loading }))(Index);
