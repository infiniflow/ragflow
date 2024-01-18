import { connect, Dispatch } from 'umi';
import { FC } from 'react'
import i18n from 'i18next';
import { useTranslation, Trans } from 'react-i18next'
import { Input, Modal, Form, Select } from 'antd'
import styles from './index.less';

type FieldType = {
    embd_id?: string;
    img2txt_id?: string;
    llm_id?: string;
    asr_id?: string
};
interface SSModalProps {
    dispatch: Dispatch;
    settingModel: any
}
const Index: FC<SSModalProps> = ({ settingModel, dispatch }) => {
    const { isShowSSModal, llmInfo = {}, tenantIfo } = settingModel

    const { t } = useTranslation()
    const handleCancel = () => {
        dispatch({
            type: 'settingModel/updateState',
            payload: {
                isShowSSModal: false
            }
        });
    };
    const [form] = Form.useForm()
    const handleOk = async () => {
        try {
            const values = await form.validateFields();
            console.log(values)
            dispatch({
                type: 'settingModel/set_tenant_info',
                payload: {
                    ...values,
                    tenant_id: tenantIfo.tenant_id,
                },
                callback: () => {
                    dispatch({
                        type: 'settingModel/updateState',
                        payload: {
                            isShowSSModal: false
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
    const handleChange = () => {

    }

    return (
        <Modal title="Basic Modal" open={isShowSSModal} onOk={handleOk} onCancel={handleCancel}>
            <Form
                form={form}
                name="validateOnly"
                // labelCol={{ span: 8 }}
                // wrapperCol={{ span: 16 }}
                style={{ maxWidth: 600 }}
                autoComplete="off"
                layout="vertical"
            >
                <Form.Item<FieldType>
                    label="embedding 模型"
                    name="embd_id"
                    rules={[{ required: true, message: 'Please input your password!' }]}
                    initialValue={tenantIfo.embd_id}

                >
                    <Select
                        // style={{ width: 200 }}
                        onChange={handleChange}
                        // fieldNames={label:}
                        options={Object.keys(llmInfo).map(t => {
                            const options = llmInfo[t].filter((d: any) => d.model_type === 'embedding').map((d: any) => ({ label: d.llm_name, value: d.llm_name, }))
                            return { label: t, options }
                        })}
                    />
                </Form.Item>
                <Form.Item<FieldType>
                    label="chat 模型"
                    name="llm_id"
                    rules={[{ required: true, message: 'Please input your password!' }]}
                    initialValue={tenantIfo.llm_id}

                >
                    <Select
                        // style={{ width: 200 }}
                        onChange={handleChange}
                        // fieldNames={label:}
                        options={Object.keys(llmInfo).map(t => {
                            const options = llmInfo[t].filter((d: any) => d.model_type === 'chat').map((d: any) => ({ label: d.llm_name, value: d.llm_name, }))
                            return { label: t, options }
                        })}
                    />
                </Form.Item>
                <Form.Item<FieldType>
                    label="image2text 模型"
                    name="img2txt_id"
                    rules={[{ required: true, message: 'Please input your password!' }]}
                    initialValue={tenantIfo.img2txt_id}

                >
                    <Select
                        // style={{ width: 200 }}
                        onChange={handleChange}
                        // fieldNames={label:}
                        options={Object.keys(llmInfo).map(t => {
                            const options = llmInfo[t].filter((d: any) => d.model_type === 'image2text').map((d: any) => ({ label: d.llm_name, value: d.llm_name, }))
                            return { label: t, options }
                        })}
                    />
                </Form.Item>
                <Form.Item<FieldType>
                    label="speech2text 模型"
                    name="asr_id"
                    rules={[{ required: true, message: 'Please input your password!' }]}
                    initialValue={tenantIfo.asr_id}

                >
                    <Select
                        // style={{ width: 200 }}
                        onChange={handleChange}
                        // fieldNames={label:}
                        options={Object.keys(llmInfo).map(t => {
                            const options = llmInfo[t].filter((d: any) => d.model_type === 'speech2text').map((d: any) => ({ label: d.llm_name, value: d.llm_name, }))
                            return { label: t, options }
                        })}
                    />
                </Form.Item>


            </Form>
        </Modal >


    );
}
export default connect(({ settingModel, loading }) => ({ settingModel, loading }))(Index);
