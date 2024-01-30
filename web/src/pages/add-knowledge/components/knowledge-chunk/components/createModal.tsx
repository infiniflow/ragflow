import { Form, Input, Modal } from 'antd';
import React, { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useDispatch } from 'umi';
import EditTag from './editTag';

type FieldType = {
  content_ltks?: string;
};
interface kFProps {
  getChunkList: () => void;
  isShowCreateModal: boolean;
  doc_id: string;
  chunk_id: string;
}

const Index: React.FC<kFProps> = ({
  getChunkList,
  doc_id,
  isShowCreateModal,
  chunk_id,
}) => {
  const dispatch = useDispatch();
  const [form] = Form.useForm();

  // const { , chunkInfo } = chunkModel
  const [important_kwd, setImportantKwd] = useState([
    'Unremovable',
    'Tag 2',
    'Tag 3',
  ]);
  const { t } = useTranslation();
  const handleCancel = () => {
    dispatch({
      type: 'chunkModel/updateState',
      payload: {
        isShowCreateModal: false,
      },
    });
  };

  const getChunk = useCallback(async () => {
    if (chunk_id && isShowCreateModal) {
      const data = await dispatch<any>({
        type: 'chunkModel/get_chunk',
        payload: {
          chunk_id,
        },
      });

      if (data?.retcode === 0) {
        const { content_ltks, important_kwd = [] } = data.data;
        form.setFieldsValue({ content_ltks });
        setImportantKwd(important_kwd);
      }
    }
  }, [chunk_id, isShowCreateModal]);

  useEffect(() => {
    getChunk();
  }, [getChunk]);

  const handleOk = async () => {
    try {
      const values = await form.validateFields();
      dispatch({
        type: 'chunkModel/create_hunk',
        payload: {
          content_ltks: values.content_ltks,
          doc_id,
          chunk_id,
          important_kwd,
        },
        // callback: () => {
        //   dispatch({
        //     type: 'chunkModel/updateState',
        //     payload: {
        //       isShowCreateModal: false,
        //     },
        //   });
        //   getChunkList && getChunkList();
        // },
      });
    } catch (errorInfo) {
      console.log('Failed:', errorInfo);
    }
  };

  return (
    <Modal
      title="Basic Modal"
      open={isShowCreateModal}
      onOk={handleOk}
      onCancel={handleCancel}
    >
      <Form
        form={form}
        name="validateOnly"
        labelCol={{ span: 5 }}
        wrapperCol={{ span: 19 }}
        style={{ maxWidth: 600 }}
        autoComplete="off"
      >
        <Form.Item<FieldType>
          label="chunk 内容"
          name="content_ltks"
          rules={[{ required: true, message: 'Please input value!' }]}
        >
          <Input.TextArea />
        </Form.Item>
        <EditTag tags={important_kwd} setTags={setImportantKwd} />
      </Form>
    </Modal>
  );
};
export default Index;
