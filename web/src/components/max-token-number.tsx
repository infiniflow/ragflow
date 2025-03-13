import { useTranslate } from '@/hooks/common-hooks';
import { Flex, Form, InputNumber, Slider } from 'antd';

interface IProps {
  initialValue?: number;
  max?: number;
}

const MaxTokenNumber = ({ initialValue = 512, max = 2048 }: IProps) => {
  const { t } = useTranslate('knowledgeConfiguration');

  return (
    <Form.Item label={t('chunkTokenNumber')} tooltip={t('chunkTokenNumberTip')}>
      <Flex gap={20} align="center">
        <Flex flex={1}>
          <Form.Item
            name={['parser_config', 'chunk_token_num']}
            noStyle
            initialValue={initialValue}
            rules={[{ required: true, message: t('chunkTokenNumberMessage') }]}
          >
            <Slider max={max} style={{ width: '100%' }} />
          </Form.Item>
        </Flex>
        <Form.Item
          name={['parser_config', 'chunk_token_num']}
          noStyle
          rules={[{ required: true, message: t('chunkTokenNumberMessage') }]}
        >
          <InputNumber max={max} min={0} />
        </Form.Item>
      </Flex>
    </Form.Item>
  );
};

export default MaxTokenNumber;
