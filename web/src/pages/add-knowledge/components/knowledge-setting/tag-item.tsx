import { useFetchKnowledgeList } from '@/hooks/knowledge-hooks';
import { UserOutlined } from '@ant-design/icons';
import { Avatar, Flex, Form, InputNumber, Select, Slider, Space } from 'antd';
import DOMPurify from 'dompurify';
import { useTranslation } from 'react-i18next';

export const TagSetItem = () => {
  const { t } = useTranslation();

  const { list: knowledgeList } = useFetchKnowledgeList(true);

  const knowledgeOptions = knowledgeList
    .filter((x) => x.parser_id === 'tag')
    .map((x) => ({
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
      label={t('knowledgeConfiguration.tagSet')}
      name={['parser_config', 'tag_kb_ids']}
      tooltip={
        <div
          dangerouslySetInnerHTML={{
            __html: DOMPurify.sanitize(t('knowledgeConfiguration.tagSetTip')),
          }}
        ></div>
      }
      rules={[
        {
          message: t('chat.knowledgeBasesMessage'),
          type: 'array',
        },
      ]}
    >
      <Select
        mode="multiple"
        options={knowledgeOptions}
        placeholder={t('chat.knowledgeBasesMessage')}
      ></Select>
    </Form.Item>
  );
};

export const TopNTagsItem = () => {
  const { t } = useTranslation();

  return (
    <Form.Item label={t('knowledgeConfiguration.topnTags')}>
      <Flex gap={20} align="center">
        <Flex flex={1}>
          <Form.Item
            name={['parser_config', 'topn_tags']}
            noStyle
            initialValue={3}
          >
            <Slider max={10} min={1} style={{ width: '100%' }} />
          </Form.Item>
        </Flex>
        <Form.Item name={['parser_config', 'topn_tags']} noStyle>
          <InputNumber max={10} min={1} />
        </Form.Item>
      </Flex>
    </Form.Item>
  );
};

export function TagItems() {
  return (
    <>
      <TagSetItem></TagSetItem>
      <Form.Item noStyle dependencies={[['parser_config', 'tag_kb_ids']]}>
        {({ getFieldValue }) => {
          const ids: string[] = getFieldValue(['parser_config', 'tag_kb_ids']);

          return (
            Array.isArray(ids) &&
            ids.length > 0 && <TopNTagsItem></TopNTagsItem>
          );
        }}
      </Form.Item>
    </>
  );
}
