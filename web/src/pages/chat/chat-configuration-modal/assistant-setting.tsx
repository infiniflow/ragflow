import KnowledgeBaseItem from '@/components/knowledge-base-item';
import { TavilyItem } from '@/components/tavily-item';
import { useTranslate } from '@/hooks/common-hooks';
import { useFetchTenantInfo } from '@/hooks/user-setting-hooks';
import { PlusOutlined } from '@ant-design/icons';
import { Form, Input, message, Select, Switch, Upload } from 'antd';
import classNames from 'classnames';
import { useCallback } from 'react';
import { ISegmentedContentProps } from '../interface';

import styles from './index.less';

const emptyResponseField = ['prompt_config', 'empty_response'];

const AssistantSetting = ({
  show,
  form,
  setHasError,
}: ISegmentedContentProps) => {
  const { t } = useTranslate('chat');
  const { data } = useFetchTenantInfo(true);

  const handleChange = useCallback(() => {
    const kbIds = form.getFieldValue('kb_ids');
    const emptyResponse = form.getFieldValue(emptyResponseField);

    const required =
      emptyResponse && ((Array.isArray(kbIds) && kbIds.length === 0) || !kbIds);

    setHasError(required);
    form.setFields([
      {
        name: emptyResponseField,
        errors: required ? [t('emptyResponseMessage')] : [],
      },
    ]);
  }, [form, setHasError, t]);

  const normFile = (e: any) => {
    if (Array.isArray(e)) {
      return e;
    }
    return e?.fileList;
  };

  const handleTtsChange = useCallback(
    (checked: boolean) => {
      if (checked && !data.tts_id) {
        message.error(`Please set TTS model firstly. 
        Setting >> Model providers >> System model settings`);
        form.setFieldValue(['prompt_config', 'tts'], false);
      }
    },
    [data, form],
  );

  const handleMemoryChange = useCallback(
    (checked: boolean) => {
      if (!checked) {
        // Reset memory config to defaults when disabled
        form.setFieldsValue({
          memory_config: {
            enabled: false,
            max_memories: 5,
            threshold: 0.7,
            store_interval: 3,
          },
        });
      }
    },
    [form],
  );

  const uploadButton = (
    <button style={{ border: 0, background: 'none' }} type="button">
      <PlusOutlined />
      <div style={{ marginTop: 8 }}>{t('upload', { keyPrefix: 'common' })}</div>
    </button>
  );

  return (
    <section
      className={classNames({
        [styles.segmentedHidden]: !show,
      })}
    >
      <Form.Item
        name={'name'}
        label={t('assistantName')}
        rules={[{ required: true, message: t('assistantNameMessage') }]}
      >
        <Input placeholder={t('namePlaceholder')} />
      </Form.Item>
      <Form.Item name={'description'} label={t('description')}>
        <Input placeholder={t('descriptionPlaceholder')} />
      </Form.Item>
      <Form.Item
        name="icon"
        label={t('assistantAvatar')}
        valuePropName="fileList"
        getValueFromEvent={normFile}
      >
        <Upload
          listType="picture-card"
          maxCount={1}
          beforeUpload={() => false}
          showUploadList={{ showPreviewIcon: false, showRemoveIcon: false }}
        >
          {show ? uploadButton : null}
        </Upload>
      </Form.Item>
      <Form.Item
        name={'language'}
        label={t('language')}
        initialValue={'English'}
        tooltip="coming soon"
        style={{ display: 'none' }}
      >
        <Select
          options={[
            { value: 'Chinese', label: t('chinese', { keyPrefix: 'common' }) },
            { value: 'English', label: t('english', { keyPrefix: 'common' }) },
          ]}
        />
      </Form.Item>
      <Form.Item
        name={emptyResponseField}
        label={t('emptyResponse')}
        tooltip={t('emptyResponseTip')}
      >
        <Input placeholder="" onChange={handleChange} />
      </Form.Item>
      <Form.Item
        name={['prompt_config', 'prologue']}
        label={t('setAnOpener')}
        tooltip={t('setAnOpenerTip')}
        initialValue={t('setAnOpenerInitial')}
      >
        <Input.TextArea autoSize={{ minRows: 5 }} />
      </Form.Item>
      <Form.Item
        label={t('quote')}
        valuePropName="checked"
        name={['prompt_config', 'quote']}
        tooltip={t('quoteTip')}
        initialValue={true}
      >
        <Switch />
      </Form.Item>
      <Form.Item
        label={t('keyword')}
        valuePropName="checked"
        name={['prompt_config', 'keyword']}
        tooltip={t('keywordTip')}
        initialValue={false}
      >
        <Switch />
      </Form.Item>
      <Form.Item
        label={t('tts')}
        valuePropName="checked"
        name={['prompt_config', 'tts']}
        tooltip={t('ttsTip')}
        initialValue={false}
      >
        <Switch onChange={handleTtsChange} />
      </Form.Item>
      <Form.Item
        label={t('memory')}
        valuePropName="checked"
        name={['memory_config', 'enabled']}
        tooltip={t('memoryTip')}
        initialValue={true}
      >
        <Switch onChange={handleMemoryChange} />
      </Form.Item>
      <Form.Item
        noStyle
        shouldUpdate={(prevValues, currentValues) =>
          prevValues.memory_config?.enabled !==
          currentValues.memory_config?.enabled
        }
      >
        {({ getFieldValue }) => {
          const memoryEnabled = getFieldValue(['memory_config', 'enabled']);
          return memoryEnabled ? (
            <>
              <Form.Item
                name={['memory_config', 'max_memories']}
                label={t('maxMemories')}
                tooltip={t('maxMemoriesTooltip')}
                initialValue={5}
                rules={[
                  { required: true, message: t('maxMemoriesRequired') },
                  {
                    type: 'number',
                    min: 1,
                    max: 50,
                    message: t('maxMemoriesRange'),
                  },
                ]}
              >
                <Input type="number" min={1} max={50} />
              </Form.Item>
              <Form.Item
                name={['memory_config', 'threshold']}
                label={t('memoryThreshold')}
                tooltip={t('memoryThresholdTooltip')}
                initialValue={0.7}
                rules={[
                  { required: true, message: t('thresholdRequired') },
                  {
                    type: 'number',
                    min: 0,
                    max: 1,
                    message: t('thresholdRange'),
                  },
                ]}
              >
                <Input type="number" step={0.1} min={0} max={1} />
              </Form.Item>
              <Form.Item
                name={['memory_config', 'store_interval']}
                label={t('storeInterval')}
                tooltip={t('storeIntervalTooltip')}
                initialValue={3}
                rules={[
                  { required: true, message: t('storeIntervalRequired') },
                  {
                    type: 'number',
                    min: 1,
                    max: 100,
                    message: t('storeIntervalRange'),
                  },
                ]}
              >
                <Input type="number" min={1} max={100} />
              </Form.Item>
            </>
          ) : null;
        }}
      </Form.Item>
      <TavilyItem></TavilyItem>
      <KnowledgeBaseItem
        required={false}
        onChange={handleChange}
      ></KnowledgeBaseItem>
    </section>
  );
};

export default AssistantSetting;
