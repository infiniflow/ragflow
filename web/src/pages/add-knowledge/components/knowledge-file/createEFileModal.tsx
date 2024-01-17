import { connect } from 'umi';
import i18n from 'i18next';
import { useTranslation, Trans } from 'react-i18next'
import { Input, Modal, Form } from 'antd'
import { rsaPsw } from '@/utils'
import styles from './index.less';

type FieldType = {
    name?: string;
};
const Index = ({ kFModel, dispatch, getKfList, kb_id }) => {
    const { isShowCEFwModal } = kFModel
    const { t } = useTranslation()
    const handleCancel = () => {
        dispatch({
            type: 'kFModel/updateState',
            payload: {
                isShowCEFwModal: false
            }
        });
    };
    const [form] = Form.useForm()
    const handleOk = async () => {
        try {
            const values = await form.validateFields();
            dispatch({
                type: 'kFModel/document_create',
                payload: {
                    name: values.name,
                    kb_id
                },
                callback: () => {
                    dispatch({
                        type: 'kFModel/updateState',
                        payload: {
                            isShowCEFwModal: false
                        }
                    });
                    getKfList && getKfList()
                }
            });

        } catch (errorInfo) {
            console.log('Failed:', errorInfo);
        }
    };

    return (
        <Modal title="Basic Modal" open={isShowCEFwModal} onOk={handleOk} onCancel={handleCancel}>
            <Form
                form={form}
                name="validateOnly"
                labelCol={{ span: 8 }}
                wrapperCol={{ span: 16 }}
                style={{ maxWidth: 600 }}
                autoComplete="off"
            >
                <Form.Item<FieldType>
                    label="文件名"
                    name="name"
                    rules={[{ required: true, message: 'Please input name!' }]}
                >
                    <Input />
                </Form.Item>

            </Form>
        </Modal >


    );
}
export default connect(({ kFModel, loading }) => ({ kFModel, loading }))(Index);
