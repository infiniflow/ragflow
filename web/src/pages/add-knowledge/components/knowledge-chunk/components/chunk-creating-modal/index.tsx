import { useDeleteChunkByIds } from '@/hooks/knowledgeHook';
import { useOneNamespaceEffectsLoading } from '@/hooks/storeHooks';
import { DeleteOutlined } from '@ant-design/icons';
import { Checkbox, Divider, Form, Input, Modal, Space } from 'antd';
import React, { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useDispatch, useSelector } from 'umi';
import EditTag from '../edit-tag';

type FieldType = {
  content?: string;
};
interface kFProps {
  doc_id: string;
  chunkId: string | undefined;
}

const ChunkCreatingModal: React.FC<kFProps> = ({ doc_id, chunkId }) => {
  const dispatch = useDispatch();
  const [form] = Form.useForm();
  const isShowCreateModal: boolean = useSelector(
    (state: any) => state.chunkModel.isShowCreateModal,
  );
  const [checked, setChecked] = useState(false);
  const [keywords, setKeywords] = useState<string[]>([]);
  const loading = useOneNamespaceEffectsLoading('chunkModel', ['create_chunk']);
  const { removeChunk } = useDeleteChunkByIds();
  const { t } = useTranslation();

  const handleCancel = () => {
    dispatch({
      type: 'chunkModel/setIsShowCreateModal',
      payload: false,
    });
  };

  const getChunk = useCallback(async () => {
    console.info(chunkId);
    if (chunkId && isShowCreateModal) {
      const data = await dispatch<any>({
        type: 'chunkModel/get_chunk',
        payload: {
          chunk_id: chunkId,
        },
      });

      if (data?.retcode === 0) {
        const { content_with_weight, important_kwd = [] } = data.data;
        form.setFieldsValue({ content: content_with_weight });
        setKeywords(important_kwd);
      }
    }

    if (!chunkId) {
      setKeywords([]);
      form.setFieldsValue({ content: undefined });
    }
  }, [chunkId, isShowCreateModal, dispatch, form]);

  useEffect(() => {
    getChunk();
  }, [getChunk]);

  const handleOk = async () => {
    try {
      const values = await form.validateFields();
      dispatch({
        type: 'chunkModel/create_chunk',
        payload: {
          content_with_weight: values.content,
          doc_id,
          chunk_id: chunkId,
          important_kwd: keywords, // keywords
        },
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
      open={isShowCreateModal}
      onOk={handleOk}
      onCancel={handleCancel}
      okButtonProps={{ loading }}
    >
      <Form
        form={form}
        // name="validateOnly"
        autoComplete="off"
        layout={'vertical'}
      >
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
