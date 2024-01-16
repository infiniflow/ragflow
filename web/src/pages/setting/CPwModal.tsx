import { connect, Dispatch } from 'umi';
import i18n from 'i18next';
import { useTranslation, Trans } from 'react-i18next'
import { Input, Modal, Form } from 'antd'
import { rsaPsw } from '@/utils'
import styles from './index.less';
import { FC } from 'react';

type FieldType = {
    newPassword?: string;
    password?: string;
};
interface CPwModalProps {
    dispatch: Dispatch;
    settingModel: any
}
const Index: FC<CPwModalProps> = ({ settingModel, dispatch }) => {
    const { isShowPSwModal } = settingModel
    const { t } = useTranslation()
    const handleCancel = () => {
        dispatch({
            type: 'settingModel/updateState',
            payload: {
                isShowPSwModal: false
            }
        });
    };
    const [form] = Form.useForm()
    const handleOk = async () => {
        try {
            const values = await form.validateFields();
            var password = rsaPsw(values.password)
            var new_password = rsaPsw(values.newPassword)

            dispatch({
                type: 'settingModel/setting',
                payload: {
                    password,
                    new_password
                },
                callback: () => {
                    dispatch({
                        type: 'settingModel/updateState',
                        payload: {
                            isShowPSwModal: false
                        }
                    });
                    dispatch({
                        type: 'settingModel/getUserInfo',
                        payload: {

                        }
                    });
                }
            });

        } catch (errorInfo) {
            console.log('Failed:', errorInfo);
        }
    };

    return (
        <Modal title="Basic Modal" open={isShowPSwModal} onOk={handleOk} onCancel={handleCancel}>
            <Form
                form={form}
                labelCol={{ span: 8 }}
                wrapperCol={{ span: 16 }}
                style={{ maxWidth: 600 }}
                autoComplete="off"
            >
                <Form.Item<FieldType>
                    label="旧密码"
                    name="password"
                    rules={[{ required: true, message: 'Please input your password!' }]}
                >
                    <Input.Password />
                </Form.Item>
                <Form.Item<FieldType>
                    label="新密码"
                    name="newPassword"
                    rules={[{ required: true, message: 'Please input your newPassword!' }]}
                >
                    <Input.Password />
                </Form.Item>

            </Form>
        </Modal >


    );
}
export default connect(({ settingModel, loading }) => ({ settingModel, loading }))(Index);
