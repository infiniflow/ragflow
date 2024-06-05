import { useTranslate } from '@/hooks/commonHooks';
import { useFetchKnowledgeList } from '@/hooks/knowledgeHook';
import { Form, Select } from 'antd';

const KnowledgeBaseItem = () => {
  const { t } = useTranslate('chat');

  const { list: knowledgeList } = useFetchKnowledgeList(true);

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
