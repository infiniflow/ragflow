import { connect, Dispatch } from 'umi';
import i18n from 'i18next';
import { FC } from 'react'
import { useTranslation, Trans } from 'react-i18next'
import { Input, Modal, Form } from 'antd'
import styles from './index.less';

type FieldType = {
    api_key?: string;
};
interface SAKModalProps {
    dispatch: Dispatch;
    settingModel: any
}
const Index: FC<SAKModalProps> = ({ settingModel, dispatch }) => {
    const { isShowSAKModal, llm_factory } = settingModel
    console.log(llm_factory)
    const { t } = useTranslation()
    const handleCancel = () => {
        dispatch({
            type: 'settingModel/updateState',
            payload: {
                isShowSAKModal: false
            }
        });
    };
    const [form] = Form.useForm()
    const handleOk = async () => {
        try {
            const values = await form.validateFields();

            dispatch({
                type: 'settingModel/set_api_key',
                payload: {
                    api_key: values.api_key,
                    llm_factory: llm_factory
                },
                callback: () => {
                    dispatch({
                        type: 'settingModel/updateState',
                        payload: {
                            isShowSAKModal: false
                        }
                    });
                    // dispatch({
                    //     type: 'settingModel/getUserInfo',
                    //     payload: {

                    //     }
                    // });
                }
            });

        } catch (errorInfo) {
            console.log('Failed:', errorInfo);
        }
    };

    return (
        <Modal title="Basic Modal" open={isShowSAKModal} onOk={handleOk} onCancel={handleCancel}>
            <Form
                form={form}
                name="validateOnly"
                labelCol={{ span: 8 }}
                wrapperCol={{ span: 16 }}
                style={{ maxWidth: 600 }}
                autoComplete="off"
            >
                <Form.Item<FieldType>
                    label="API Key"
                    name="api_key"
                    rules={[{ required: true, message: 'Please input ' }]}
                >
                    <Input />
                </Form.Item>

            </Form>
        </Modal >


    );
}
export default connect(({ settingModel, loading }) => ({ settingModel, loading }))(Index);
