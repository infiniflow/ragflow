import { useTranslate } from '@/hooks/common-hooks';
import { Flex, Form, InputNumber, Slider } from 'antd';

export const AutoKeywordsItem = () => {
  const { t } = useTranslate('knowledgeDetails');

  return (
    <Form.Item label={t('autoKeywords')} tooltip={t('autoKeywordsTip')}>
      <Flex gap={20} align="center">
        <Flex flex={1}>
          <Form.Item
            name={['parser_config', 'auto_keywords']}
            noStyle
            initialValue={0}
          >
            <Slider max={30} style={{ width: '100%' }} />
          </Form.Item>
        </Flex>
        <Form.Item name={['parser_config', 'auto_keywords']} noStyle>
          <InputNumber max={30} min={0} />
        </Form.Item>
      </Flex>
    </Form.Item>
  );
};

export const AutoQuestionsItem = () => {
  const { t } = useTranslate('knowledgeDetails');

  return (
    <Form.Item label={t('autoQuestions')} tooltip={t('autoQuestionsTip')}>
      <Flex gap={20} align="center">
        <Flex flex={1}>
          <Form.Item
            name={['parser_config', 'auto_questions']}
            noStyle
            initialValue={0}
          >
            <Slider max={10} style={{ width: '100%' }} />
          </Form.Item>
        </Flex>
        <Form.Item name={['parser_config', 'auto_questions']} noStyle>
          <InputNumber max={10} min={0} />
        </Form.Item>
      </Flex>
    </Form.Item>
  );
};
