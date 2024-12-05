import { useTranslate } from '@/hooks/common-hooks';
import { useFetchKnowledgeList } from '@/hooks/knowledge-hooks';
import { UserOutlined } from '@ant-design/icons';
import { Avatar, Form, Select, Space } from 'antd';

const KnowledgeBaseItem = () => {
  const { t } = useTranslate('chat');

  const { list: knowledgeList } = useFetchKnowledgeList(true);

  const knowledgeOptions = knowledgeList.map((x) => ({
    label: (
      <Space>
        <Avatar size={20} icon={<UserOutlined />} src={x.avatar} />
        {x.name}
      </Space>
    ),
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
