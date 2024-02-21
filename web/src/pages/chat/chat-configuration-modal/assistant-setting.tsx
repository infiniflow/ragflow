import { Form, Input, Select } from 'antd';

import classNames from 'classnames';
import { ISegmentedContentProps } from './interface';

import { useFetchKnowledgeList } from '@/hooks/knowledgeHook';
import styles from './index.less';

const AssistantSetting = ({ show }: ISegmentedContentProps) => {
  const knowledgeList = useFetchKnowledgeList();
  const knowledgeOptions = knowledgeList.map((x) => ({
    label: x.name,
    value: x.id,
  }));

  return (
    <section
      className={classNames({
        [styles.segmentedHidden]: !show,
      })}
    >
      <Form.Item
        name={'name'}
        label="Assistant name"
        rules={[{ required: true }]}
      >
        <Input placeholder="e.g. Resume Jarvis" />
      </Form.Item>
      <Form.Item name={'icon'} label="Assistant avatar">
        <Input />
      </Form.Item>
      <Form.Item name={'language'} label="Language" initialValue={'Chinese'}>
        <Select
          options={[
            { value: 'Chinese', label: 'Chinese' },
            { value: 'English', label: 'English' },
          ]}
        />
      </Form.Item>
      <Form.Item
        name={['prompt_config', 'empty_response']}
        label="Empty response"
      >
        <Input placeholder="" />
      </Form.Item>
      <Form.Item name={['prompt_config', 'prologue']} label="Set an opener">
        <Input.TextArea autoSize={{ minRows: 5 }} />
      </Form.Item>
      <Form.Item
        label="Select one context"
        name="kb_ids"
        rules={[
          {
            required: true,
            message: 'Please select!',
            type: 'array',
          },
        ]}
      >
        <Select
          mode="multiple"
          options={knowledgeOptions}
          placeholder="Please select"
        ></Select>
      </Form.Item>
    </section>
  );
};

export default AssistantSetting;
