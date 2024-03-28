import { Flex, Form, InputNumber, Slider } from 'antd';

const MaxTokenNumber = () => {
  return (
    <Form.Item
      label="Chunk token number"
      tooltip="It determine the token number of a chunk approximately."
    >
      <Flex gap={20} align="center">
        <Flex flex={1}>
          <Form.Item
            name={['parser_config', 'chunk_token_num']}
            noStyle
            initialValue={128}
            rules={[{ required: true, message: 'Province is required' }]}
          >
            <Slider max={2048} style={{ width: '100%' }} />
          </Form.Item>
        </Flex>
        <Form.Item
          name={['parser_config', 'chunk_token_num']}
          noStyle
          rules={[{ required: true, message: 'Street is required' }]}
        >
          <InputNumber max={2048} min={0} />
        </Form.Item>
      </Flex>
    </Form.Item>
  );
};

export default MaxTokenNumber;
