import { useTranslate } from '@/hooks/common-hooks';
import { Form } from 'antd';
import EditTag from './edit-tag';

const initialEntityTypes = [
  'organization',
  'person',
  'geo',
  'event',
  'category',
];

type IProps = {
  field?: string[];
};

const EntityTypesItem = ({
  field = ['parser_config', 'entity_types'],
}: IProps) => {
  const { t } = useTranslate('knowledgeConfiguration');
  return (
    <Form.Item
      name={field}
      label={t('entityTypes')}
      rules={[{ required: true }]}
      initialValue={initialEntityTypes}
    >
      <EditTag></EditTag>
    </Form.Item>
  );
};

export default EntityTypesItem;
