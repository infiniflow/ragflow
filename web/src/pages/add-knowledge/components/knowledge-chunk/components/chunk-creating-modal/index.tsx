import EditTag from '@/components/edit-tag';
import { useFetchChunk } from '@/hooks/chunk-hooks';
import { IModalProps } from '@/interfaces/common';
import { DeleteOutlined } from '@ant-design/icons';
import { Divider, Form, Input, Modal, Space, Switch } from 'antd';
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
    if (data?.code === 0) {
      const {
        content_with_weight,
        important_kwd = [],
        available_int,
      } = data.data;
      form.setFieldsValue({ content: content_with_weight });
      setKeywords(important_kwd);
      setChecked(available_int === 1);
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
        available_int: checked ? 1 : 0, // available_int
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
        <p className="mb-2">{t('chunk.keyword')} *</p>
        <EditTag tags={keywords} setTags={setKeywords} />
      </section>
      {chunkId && (
        <section>
          <Divider></Divider>
          <Space size={'large'}>
            <Switch
              checkedChildren={t('chunk.enabled')}
              unCheckedChildren={t('chunk.disabled')}
              onChange={handleCheck}
              checked={checked}
            />

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
