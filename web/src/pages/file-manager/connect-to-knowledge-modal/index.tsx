import { useTranslate } from '@/hooks/common-hooks';
import { useFetchKnowledgeList } from '@/hooks/knowledge-hooks';
import { IModalProps } from '@/interfaces/common';
import { filterOptionsByInput } from '@/utils/common-util';
import { Form, Modal, Select } from 'antd';
import { useEffect } from 'react';

const ConnectToKnowledgeModal = ({
  visible,
  hideModal,
  onOk,
  initialValue,
  loading,
}: IModalProps<string[]> & { initialValue: string[] }) => {
  const [form] = Form.useForm();
  const { list } = useFetchKnowledgeList();
  const { t } = useTranslate('fileManager');

  const options = list?.map((item) => ({
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
    }
  }, [visible, initialValue, form]);

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
            showSearch
            style={{ width: '100%' }}
            placeholder={t('pleaseSelect')}
            options={options}
            optionFilterProp="children"
            filterOption={filterOptionsByInput}
          />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default ConnectToKnowledgeModal;
