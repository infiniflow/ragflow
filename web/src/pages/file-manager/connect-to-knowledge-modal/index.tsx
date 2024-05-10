import { useTranslate } from '@/hooks/commonHooks';
import { useFetchKnowledgeList } from '@/hooks/knowledgeHook';
import { IModalProps } from '@/interfaces/common';
import { Form, Modal, Select, SelectProps } from 'antd';
import { useEffect } from 'react';

const ConnectToKnowledgeModal = ({
  visible,
  hideModal,
  onOk,
  initialValue,
  loading,
}: IModalProps<string[]> & { initialValue: string[] }) => {
  const [form] = Form.useForm();
  const { list, fetchList } = useFetchKnowledgeList();
  const { t } = useTranslate('fileManager');

  const options: SelectProps['options'] = list?.map((item) => ({
    label: item.name,
    value: item.id,
  }));

  const handleOk = async () => {
    const values = await form.getFieldsValue();
    const knowledgeIds = values.knowledgeIds ?? [];
    return onOk?.(knowledgeIds);
  };

  useEffect(() => {
    if (visible) {
      form.setFieldValue('knowledgeIds', initialValue);
      fetchList();
    }
  }, [visible, fetchList, initialValue, form]);

  return (
    <Modal
      title={t('addToKnowledge')}
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
      confirmLoading={loading}
    >
      <Form form={form}>
        <Form.Item name="knowledgeIds" noStyle>
          <Select
            mode="multiple"
            allowClear
            style={{ width: '100%' }}
            placeholder={t('pleaseSelect')}
            options={options}
          />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default ConnectToKnowledgeModal;
