import EditTag from '@/components/edit-tag';
import { useFetchChunk } from '@/hooks/chunk-hooks';
import { IModalProps } from '@/interfaces/common';
import { IChunk } from '@/interfaces/database/knowledge';
import { DeleteOutlined } from '@ant-design/icons';
import { Divider, Form, Input, Modal, Space, Switch } from 'antd';
import React, { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useDeleteChunkByIds } from '../../hooks';
import {
  transformTagFeaturesArrayToObject,
  transformTagFeaturesObjectToArray,
} from '../../utils';
import { TagFeatureItem } from './tag-feature-item';

type FieldType = Pick<
  IChunk,
  'content_with_weight' | 'tag_kwd' | 'question_kwd' | 'important_kwd'
>;

interface kFProps {
  doc_id: string;
  chunkId: string | undefined;
  parserId: string;
}

const ChunkCreatingModal: React.FC<IModalProps<any> & kFProps> = ({
  doc_id,
  chunkId,
  hideModal,
  onOk,
  loading,
  parserId,
}) => {
  const [form] = Form.useForm();
  const [checked, setChecked] = useState(false);
  const { removeChunk } = useDeleteChunkByIds();
  const { data } = useFetchChunk(chunkId);
  const { t } = useTranslation();

  const isTagParser = parserId === 'tag';

  const handleOk = useCallback(async () => {
    try {
      const values = await form.validateFields();
      console.log('🚀 ~ handleOk ~ values:', values);

      onOk?.({
        ...values,
        tag_feas: transformTagFeaturesArrayToObject(values.tag_feas),
        available_int: checked ? 1 : 0, // available_int
      });
    } catch (errorInfo) {
      console.log('Failed:', errorInfo);
    }
  }, [checked, form, onOk]);

  const handleRemove = useCallback(() => {
    if (chunkId) {
      return removeChunk([chunkId], doc_id);
    }
  }, [chunkId, doc_id, removeChunk]);

  const handleCheck = useCallback(() => {
    setChecked(!checked);
  }, [checked]);

  useEffect(() => {
    if (data?.code === 0) {
      const { available_int, tag_feas } = data.data;
      form.setFieldsValue({
        ...(data.data || {}),
        tag_feas: transformTagFeaturesObjectToArray(tag_feas),
      });

      setChecked(available_int !== 0);
    }
  }, [data, form, chunkId]);

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
          name="content_with_weight"
          rules={[{ required: true, message: t('chunk.chunkMessage') }]}
        >
          <Input.TextArea autoSize={{ minRows: 4, maxRows: 10 }} />
        </Form.Item>

        <Form.Item<FieldType> label={t('chunk.keyword')} name="important_kwd">
          <EditTag></EditTag>
        </Form.Item>
        <Form.Item<FieldType>
          label={t('chunk.question')}
          name="question_kwd"
          tooltip={t('chunk.questionTip')}
        >
          <EditTag></EditTag>
        </Form.Item>
        {isTagParser && (
          <Form.Item<FieldType>
            label={t('knowledgeConfiguration.tagName')}
            name="tag_kwd"
          >
            <EditTag></EditTag>
          </Form.Item>
        )}

        {!isTagParser && <TagFeatureItem></TagFeatureItem>}
      </Form>

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
