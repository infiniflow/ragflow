import EditTag from '@/components/edit-tag';
import { useFetchChunk } from '@/hooks/chunk-hooks';
import { IModalProps } from '@/interfaces/common';
import { DeleteOutlined } from '@ant-design/icons';
import { Checkbox, Divider, Form, Input, Modal, Space } from 'antd';
import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useDeleteChunkByIds } from '../../hooks';

type FieldType = {
  content?: string;
};
interface kFProps {
  doc_id: string;
  chunkId: string | undefined;
}

const ChunkCreatingModal: React.FC<IModalProps<any> & kFProps> = ({
  doc_id,
  chunkId,
  hideModal,
  onOk,
  loading,
}) => {
  const [form] = Form.useForm();
  const [checked, setChecked] = useState(false);
  const [keywords, setKeywords] = useState<string[]>([]);
  const { removeChunk } = useDeleteChunkByIds();
  const { data } = useFetchChunk(chunkId);
  const { t } = useTranslation();

  useEffect(() => {
    if (data?.retcode === 0) {
      const { content_with_weight, important_kwd = [] } = data.data;
      form.setFieldsValue({ content: content_with_weight });
      setKeywords(important_kwd);
    }

    if (!chunkId) {
      setKeywords([]);
      form.setFieldsValue({ content: undefined });
    }
  }, [data, form, chunkId]);

  const handleOk = async () => {
    try {
      const values = await form.validateFields();
      onOk?.({
        content: values.content,
        keywords, // keywords
      });
    } catch (errorInfo) {
      console.log('Failed:', errorInfo);
    }
  };

  const handleRemove = () => {
    if (chunkId) {
      return removeChunk([chunkId], doc_id);
    }
  };
  const handleCheck = () => {
    setChecked(!checked);
  };

  return (
    <Modal
      title={`${chunkId ? t('common.edit') : t('common.create')} ${t('chunk.chunk')}`}
      open={true}
      onOk={handleOk}
      onCancel={hideModal}
      okButtonProps={{ loading }}
      destroyOnClose
    >
      <Form form={form} autoComplete="off" layout={'vertical'}>
        <Form.Item<FieldType>
          label={t('chunk.chunk')}
          name="content"
          rules={[{ required: true, message: t('chunk.chunkMessage') }]}
        >
          <Input.TextArea autoSize={{ minRows: 4, maxRows: 10 }} />
        </Form.Item>
      </Form>
      <section>
        <p>{t('chunk.keyword')} *</p>
        <EditTag tags={keywords} setTags={setKeywords} />
      </section>
      {chunkId && (
        <section>
          <Divider></Divider>
          <Space size={'large'}>
            <Checkbox onChange={handleCheck} checked={checked}>
              {t('chunk.enabled')}
            </Checkbox>

            <span onClick={handleRemove}>
              <DeleteOutlined /> {t('common.delete')}
            </span>
          </Space>
        </section>
      )}
    </Modal>
  );
};
export default ChunkCreatingModal;
