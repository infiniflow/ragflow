import { normFile } from '@/utils/fileUtil';
import { PlusOutlined } from '@ant-design/icons';
import { Button, Form, Input, Radio, Select, Space, Upload } from 'antd';
import {
  useFetchKnowledgeConfigurationOnMount,
  useSubmitKnowledgeConfiguration,
} from './hooks';

import MaxTokenNumber from '@/components/max-token-number';
import { useTranslate } from '@/hooks/commonHooks';
import { FormInstance } from 'antd/lib';
import styles from './index.less';

const { Option } = Select;

const ConfigurationForm = ({ form }: { form: FormInstance }) => {
  const { submitKnowledgeConfiguration, submitLoading, navigateToDataset } =
    useSubmitKnowledgeConfiguration(form);
  const { parserList, embeddingModelOptions, disabled } =
    useFetchKnowledgeConfigurationOnMount(form);
  const { t } = useTranslate('knowledgeConfiguration');

  return (
    <Form form={form} name="validateOnly" layout="vertical" autoComplete="off">
      <Form.Item name="name" label={t('name')} rules={[{ required: true }]}>
        <Input />
      </Form.Item>
      <Form.Item
        name="avatar"
        label={t('photo')}
        valuePropName="fileList"
        getValueFromEvent={normFile}
      >
        <Upload
          listType="picture-card"
          maxCount={1}
          beforeUpload={() => false}
          showUploadList={{ showPreviewIcon: false, showRemoveIcon: false }}
        >
          <button style={{ border: 0, background: 'none' }} type="button">
            <PlusOutlined />
            <div style={{ marginTop: 8 }}>{t('upload')}</div>
          </button>
        </Upload>
      </Form.Item>
      <Form.Item name="description" label={t('description')}>
        <Input />
      </Form.Item>
      <Form.Item
        label={t('language')}
        name="language"
        initialValue={'English'}
        rules={[{ required: true, message: t('languageMessage') }]}
      >
        <Select placeholder={t('languagePlaceholder')}>
          <Option value="English">{t('english')}</Option>
          <Option value="Chinese">{t('chinese')}</Option>
        </Select>
      </Form.Item>
      <Form.Item
        name="permission"
        label="Permissions"
        tooltip={t('permissionsTip')}
        rules={[{ required: true }]}
      >
        <Radio.Group>
          <Radio value="me">{t('me')}</Radio>
          <Radio value="team">{t('team')}</Radio>
        </Radio.Group>
      </Form.Item>
      <Form.Item
        name="embd_id"
        label="Embedding model"
        rules={[{ required: true }]}
        tooltip={t('embeddingModelTip')}
      >
        <Select
          placeholder={t('embeddingModelPlaceholder')}
          options={embeddingModelOptions}
          disabled={disabled}
        ></Select>
      </Form.Item>
      <Form.Item
        name="parser_id"
        label="Chunk method"
        tooltip={t('chunkMethodTip')}
        rules={[{ required: true }]}
      >
        <Select placeholder={t('chunkMethodPlaceholder')} disabled={disabled}>
          {parserList.map((x) => (
            <Option value={x.value} key={x.value}>
              {x.label}
            </Option>
          ))}
        </Select>
      </Form.Item>
      <Form.Item noStyle dependencies={['parser_id']}>
        {({ getFieldValue }) => {
          const parserId = getFieldValue('parser_id');

          if (parserId === 'naive') {
            return <MaxTokenNumber></MaxTokenNumber>;
          }
          return null;
        }}
      </Form.Item>
      <Form.Item>
        <div className={styles.buttonWrapper}>
          <Space>
            <Button size={'middle'} onClick={navigateToDataset}>
              {t('cancel')}
            </Button>
            <Button
              type="primary"
              size={'middle'}
              loading={submitLoading}
              onClick={submitKnowledgeConfiguration}
            >
              {t('save')}
            </Button>
          </Space>
        </div>
      </Form.Item>
    </Form>
  );
};

export default ConfigurationForm;
