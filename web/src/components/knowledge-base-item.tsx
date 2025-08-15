import { DocumentParserType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import { useFetchKnowledgeList } from '@/hooks/knowledge-hooks';
import { useBuildQueryVariableOptions } from '@/pages/agent/hooks/use-get-begin-query';
import { UserOutlined } from '@ant-design/icons';
import { Avatar as AntAvatar, Form, Select, Space } from 'antd';
import { toLower } from 'lodash';
import { useMemo } from 'react';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { RAGFlowAvatar } from './ragflow-avatar';
import { FormControl, FormField, FormItem, FormLabel } from './ui/form';
import { MultiSelect } from './ui/multi-select';

interface KnowledgeBaseItemProps {
  label?: string;
  tooltipText?: string;
  name?: string;
  required?: boolean;
  onChange?(): void;
}

const KnowledgeBaseItem = ({
  label,
  tooltipText,
  name,
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
      label={label || t('knowledgeBases')}
      name={name || 'kb_ids'}
      tooltip={tooltipText || t('knowledgeBasesTip')}
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

function buildQueryVariableOptionsByShowVariable(showVariable?: boolean) {
  return showVariable ? useBuildQueryVariableOptions : () => [];
}

export function KnowledgeBaseFormField({
  showVariable = false,
}: {
  showVariable?: boolean;
}) {
  const form = useFormContext();
  const { t } = useTranslation();

  const { list: knowledgeList } = useFetchKnowledgeList(true);

  const filteredKnowledgeList = knowledgeList.filter(
    (x) => x.parser_id !== DocumentParserType.Tag,
  );

  const nextOptions = buildQueryVariableOptionsByShowVariable(showVariable)();

  const knowledgeOptions = filteredKnowledgeList.map((x) => ({
    label: x.name,
    value: x.id,
    icon: () => (
      <RAGFlowAvatar className="size-4 mr-2" avatar={x.avatar} name={x.name} />
    ),
  }));

  const options = useMemo(() => {
    if (showVariable) {
      return [
        {
          label: t('knowledgeDetails.dataset'),
          options: knowledgeOptions,
        },
        ...nextOptions.map((x) => {
          return {
            ...x,
            options: x.options
              .filter((y) => toLower(y.type).includes('string'))
              .map((x) => ({
                ...x,
                icon: () => (
                  <RAGFlowAvatar
                    className="size-4 mr-2"
                    avatar={x.label}
                    name={x.label}
                  />
                ),
              })),
          };
        }),
      ];
    }

    return knowledgeOptions;
  }, [knowledgeOptions, nextOptions, showVariable, t]);

  return (
    <FormField
      control={form.control}
      name="kb_ids"
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t('chat.knowledgeBases')}</FormLabel>
          <FormControl>
            <MultiSelect
              options={options}
              onValueChange={field.onChange}
              placeholder={t('chat.knowledgeBasesMessage')}
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
