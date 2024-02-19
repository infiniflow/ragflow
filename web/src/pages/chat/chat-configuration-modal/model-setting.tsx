import { Divider, Form, Select } from 'antd';
import { ISegmentedContentProps } from './interface';

const ModelSetting = ({ show }: ISegmentedContentProps) => {
  return (
    <>
      <Form.Item
        label="Model"
        name="model"
        hidden={!show}
        rules={[{ required: true, message: 'Please input!' }]}
      >
        <Select />
      </Form.Item>
      <Divider></Divider>
      <Form.Item
        label="Parameters"
        name="parameters"
        hidden={!show}
        rules={[{ required: true, message: 'Please input!' }]}
      >
        <Select />
      </Form.Item>
    </>
  );
};

export default ModelSetting;
