import { useTranslate } from '@/hooks/common-hooks';
import { Form } from 'antd';
import EditTag from './edit-tag';

const initialEntityTypes = [
  'organization',
  'person',
  'location',
  'event',
  'time',
];

const EntityTypesItem = () => {
  const { t } = useTranslate('knowledgeConfiguration');
  return (
    <Form.Item
      name={['parser_config', 'entity_types']}
      label={t('entityTypes')}
      rules={[{ required: true }]}
      initialValue={initialEntityTypes}
      valuePropName="tags"
      trigger="setTags"
    >
      <EditTag></EditTag>
    </Form.Item>
  );
};

export default EntityTypesItem;
