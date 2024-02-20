import { Form, Input, Select } from 'antd';

import classNames from 'classnames';
import { ISegmentedContentProps } from './interface';

import styles from './index.less';

const { Option } = Select;

const AssistantSetting = ({ show }: ISegmentedContentProps) => {
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
      <Form.Item name={'avatar'} label="Assistant avatar">
        <Input />
      </Form.Item>
      <Form.Item name={'language'} label="Language">
        <Select
          defaultValue="english"
          options={[
            { value: 'english', label: 'english' },
            { value: 'chinese', label: 'chinese' },
          ]}
        />
      </Form.Item>
      <Form.Item name={'opener'} label="Set an opener">
        <Input.TextArea autoSize={{ minRows: 5 }} />
      </Form.Item>
      <Form.Item
        label="Select one context"
        name="context"
        rules={[
          {
            required: true,
            message: 'Please select your favourite colors!',
            type: 'array',
          },
        ]}
      >
        <Select mode="multiple" placeholder="Please select favourite colors">
          <Option value="red">Red</Option>
          <Option value="green">Green</Option>
          <Option value="blue">Blue</Option>
        </Select>
      </Form.Item>
    </section>
  );
};

export default AssistantSetting;
