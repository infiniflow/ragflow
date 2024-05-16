import { useFetchKnowledgeList } from '@/hooks/knowledgeHook';
import { PlusOutlined } from '@ant-design/icons';
import { Form, Input, Select, Switch, Upload } from 'antd';
import classNames from 'classnames';
import { ISegmentedContentProps } from '../interface';

import { useTranslate } from '@/hooks/commonHooks';
import styles from './index.less';

const AssistantSetting = ({ show }: ISegmentedContentProps) => {
  const { list: knowledgeList } = useFetchKnowledgeList(true);
  const knowledgeOptions = knowledgeList.map((x) => ({
    label: x.name,
    value: x.id,
  }));
  const { t } = useTranslate('chat');

  const normFile = (e: any) => {
    if (Array.isArray(e)) {
      return e;
    }
    return e?.fileList;
  };

  const uploadButtion = (
    <button style={{ border: 0, background: 'none' }} type="button">
            <PlusOutlined />
            <div style={{ marginTop: 8 }}>
              {t('upload', { keyPrefix: 'common' })}
            </div>
          </button>
  )

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
      <Form.Item
        name="icon"
        label={t('assistantAvatar')}
        valuePropName="fileList"
        getValueFromEvent={normFile}
      >
        <Upload
          listType="picture-card"
          maxCount={1}
          showUploadList={{ showPreviewIcon: false, showRemoveIcon: false }}
        >
          {show ? uploadButtion : null}
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
        name={['prompt_config', 'empty_response']}
        label={t('emptyResponse')}
        tooltip={t('emptyResponseTip')}
      >
        <Input placeholder="" />
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
        label={t('knowledgeBases')}
        name="kb_ids"
        tooltip={t('knowledgeBasesTip')}
        rules={[
          {
            required: true,
            message: t('knowledgeBasesMessage'),
            type: 'array',
          },
        ]}
      >
        <Select
          mode="multiple"
          options={knowledgeOptions}
          placeholder={t('knowledgeBasesMessage')}
        ></Select>
      </Form.Item>
    </section>
  );
};

export default AssistantSetting;
