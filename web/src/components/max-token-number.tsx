import { useTranslate } from '@/hooks/common-hooks';
import { Flex, Form, InputNumber, Slider } from 'antd';

const MaxTokenNumber = () => {
  const { t } = useTranslate('knowledgeConfiguration');

  return (
    <Form.Item label={t('chunkTokenNumber')} tooltip={t('chunkTokenNumberTip')}>
      <Flex gap={20} align="center">
        <Flex flex={1}>
          <Form.Item
            name={['parser_config', 'chunk_token_num']}
            noStyle
            initialValue={128}
            rules={[{ required: true, message: t('chunkTokenNumberMessage') }]}
          >
            <Slider max={2048} style={{ width: '100%' }} />
          </Form.Item>
        </Flex>
        <Form.Item
          name={['parser_config', 'chunk_token_num']}
          noStyle
          rules={[{ required: true, message: t('chunkTokenNumberMessage') }]}
        >
          <InputNumber max={2048} min={0} />
        </Form.Item>
      </Flex>
    </Form.Item>
  );
};

export default MaxTokenNumber;
