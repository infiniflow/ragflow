import { DocumentParserType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import { useFetchKnowledgeList } from '@/hooks/knowledge-hooks';
import { UserOutlined } from '@ant-design/icons';
import { Avatar as AntAvatar, Form, Select, Space } from 'antd';
import { Book } from 'lucide-react';
import { useFormContext } from 'react-hook-form';
import { Avatar, AvatarFallback, AvatarImage } from './ui/avatar';
import { FormControl, FormField, FormItem, FormLabel } from './ui/form';
import { MultiSelect } from './ui/multi-select';

interface KnowledgeBaseItemProps {
  required?: boolean;
  onChange?(): void;
}

const KnowledgeBaseItem = ({
  required = true,
  onChange,
}: KnowledgeBaseItemProps) => {
  const { t } = useTranslate('chat');

  const { list: knowledgeList } = useFetchKnowledgeList(true);

  const filteredKnowledgeList = knowledgeList.filter(
    (x) => x.parser_id !== DocumentParserType.Tag,
  );

  const knowledgeOptions = filteredKnowledgeList.map((x) => ({
    label: (
      <Space>
        <AntAvatar size={20} icon={<UserOutlined />} src={x.avatar} />
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
          required,
          message: t('knowledgeBasesMessage'),
          type: 'array',
        },
      ]}
    >
      <Select
        mode="multiple"
        options={knowledgeOptions}
        placeholder={t('knowledgeBasesMessage')}
        onChange={onChange}
      ></Select>
    </Form.Item>
  );
};

export default KnowledgeBaseItem;

export function KnowledgeBaseFormField() {
  const form = useFormContext();
  const { t } = useTranslate('chat');

  const { list: knowledgeList } = useFetchKnowledgeList(true);

  const filteredKnowledgeList = knowledgeList.filter(
    (x) => x.parser_id !== DocumentParserType.Tag,
  );

  const knowledgeOptions = filteredKnowledgeList.map((x) => ({
    label: x.name,
    value: x.id,
    icon: () => (
      <Avatar className="size-4 mr-2">
        <AvatarImage src={x.avatar} />
        <AvatarFallback>
          <Book />
        </AvatarFallback>
      </Avatar>
    ),
  }));

  return (
    <FormField
      control={form.control}
      name="kb_ids"
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t('knowledgeBases')}</FormLabel>
          <FormControl>
            <MultiSelect
              options={knowledgeOptions}
              onValueChange={field.onChange}
              placeholder={t('knowledgeBasesMessage')}
              variant="inverted"
              maxCount={100}
              defaultValue={field.value}
              {...field}
            />
          </FormControl>
        </FormItem>
      )}
    />
  );
}
