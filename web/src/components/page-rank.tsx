import { useTranslate } from '@/hooks/common-hooks';
import { Flex, Form, InputNumber, Slider } from 'antd';

const PageRank = () => {
  const { t } = useTranslate('knowledgeConfiguration');

  return (
    <Form.Item label={t('pageRank')} tooltip={t('pageRankTip')}>
      <Flex gap={20} align="center">
        <Flex flex={1}>
          <Form.Item
            name={['pagerank']}
            noStyle
            initialValue={0}
            rules={[{ required: true }]}
          >
            <Slider max={100} style={{ width: '100%' }} />
          </Form.Item>
        </Flex>
        <Form.Item name={['pagerank']} noStyle rules={[{ required: true }]}>
          <InputNumber max={100} min={0} />
        </Form.Item>
      </Flex>
    </Form.Item>
  );
};

export default PageRank;
