import { Form, Input, InputNumber } from 'antd';

import { ISegmentedContentProps } from './interface';

const AssistantSetting = ({ show }: ISegmentedContentProps) => {
  return (
    <>
      <Form.Item
        name={['user', 'name']}
        label="Name"
        hidden={!show}
        rules={[{ required: true }]}
      >
        <Input />
      </Form.Item>
      <Form.Item
        name={['user', 'email']}
        label="Email"
        rules={[{ type: 'email' }]}
        hidden={!show}
      >
        <Input />
      </Form.Item>
      <Form.Item
        name={['user', 'age']}
        label="Age"
        hidden={!show}
        rules={[{ type: 'number', min: 0, max: 99 }]}
      >
        <InputNumber />
      </Form.Item>
      <Form.Item name={['user', 'website']} label="Website" hidden={!show}>
        <Input />
      </Form.Item>
      <Form.Item
        name={['user', 'introduction']}
        label="Introduction"
        hidden={!show}
      >
        <Input.TextArea />
      </Form.Item>
    </>
  );
};

export default AssistantSetting;
