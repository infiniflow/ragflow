import { useTranslate } from '@/hooks/common-hooks';
import { useNextFetchKnowledgeList } from '@/hooks/knowledge-hooks';
import { Form, Select } from 'antd';

const KnowledgeBaseItem = () => {
  const { t } = useTranslate('chat');

  const { list: knowledgeList } = useNextFetchKnowledgeList(true);

  const knowledgeOptions = knowledgeList.map((x) => ({
    label: x.name,
    value: x.id,
  }));

  return (
    <Form.Item
      label={t('knowledgeBases')}
      name="kb_ids"
      tooltip={t('knowledgeBasesTip')}
      rules={[
        {
          required: true,
          message: t('knowledgeBasesMessage'),
          type: 'array',
        },
      ]}
    >
      <Select
        mode="multiple"
        options={knowledgeOptions}
        placeholder={t('knowledgeBasesMessage')}
      ></Select>
    </Form.Item>
  );
};

export default KnowledgeBaseItem;
