import { useTranslate } from '@/hooks/common-hooks';
import { Form, InputNumber } from 'antd';

const style = {
  width: '100%',
};

export const AutoKeywordsItem = () => {
  const { t } = useTranslate('knowledgeDetails');

  return (
    <Form.Item
      label={t('autoKeywords')}
      name={['parser_config', 'auto_keywords']}
      tooltip={t('autoKeywordsTip')}
      initialValue={0}
    >
      <InputNumber style={style}></InputNumber>
    </Form.Item>
  );
};

export const AutoQuestionsItem = () => {
  const { t } = useTranslate('knowledgeDetails');

  return (
    <Form.Item
      label={t('autoQuestions')}
      name={['parser_config', 'auto_questions']}
      tooltip={t('autoQuestionsTip')}
      initialValue={0}
    >
      <InputNumber style={style}></InputNumber>
    </Form.Item>
  );
};
