import { IModalProps } from '@/interfaces/common';
import { IDocumentInfo } from '@/interfaces/database/document';
import Editor, { loader } from '@monaco-editor/react';

import { Form, Modal } from 'antd';
import DOMPurify from 'dompurify';
import { useCallback, useEffect } from 'react';
import { useTranslation } from 'react-i18next';

loader.config({ paths: { vs: '/vs' } });

type FieldType = {
  meta?: string;
};

export function SetMetaModal({
  visible,
  hideModal,
  onOk,
  initialMetaData,
}: IModalProps<any> & { initialMetaData?: IDocumentInfo['meta_fields'] }) {
  const { t } = useTranslation();
  const [form] = Form.useForm();

  const handleOk = useCallback(async () => {
    const values = await form.validateFields();
    onOk?.(values.meta);
  }, [form, onOk]);

  useEffect(() => {
    form.setFieldValue('meta', JSON.stringify(initialMetaData, null, 4));
  }, [form, initialMetaData]);

  return (
    <Modal
      title={t('knowledgeDetails.setMetaData')}
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
    >
      <Form
        name="basic"
        initialValues={{ remember: true }}
        autoComplete="off"
        layout={'vertical'}
        form={form}
      >
        <Form.Item<FieldType>
          label={t('knowledgeDetails.metaData')}
          name="meta"
          rules={[
            {
              required: true,
              validator(rule, value) {
                try {
                  JSON.parse(value);
                  return Promise.resolve();
                } catch (error) {
                  return Promise.reject(
                    new Error(t('knowledgeDetails.pleaseInputJson')),
                  );
                }
              },
            },
          ]}
          tooltip={
            <div
              dangerouslySetInnerHTML={{
                __html: DOMPurify.sanitize(
                  t('knowledgeDetails.documentMetaTips'),
                ),
              }}
            ></div>
          }
        >
          <Editor height={200} defaultLanguage="json" theme="vs-dark" />
        </Form.Item>
      </Form>
    </Modal>
  );
}
