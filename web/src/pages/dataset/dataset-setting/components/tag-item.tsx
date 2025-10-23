import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { SliderInputFormField } from '@/components/slider-input-form-field';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { MultiSelect } from '@/components/ui/multi-select';
import { FormLayout } from '@/constants/form';
import { useFetchKnowledgeList } from '@/hooks/knowledge-hooks';
import { Form, Select, Space } from 'antd';
import DOMPurify from 'dompurify';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

export const TagSetItem = () => {
  const { t } = useTranslation();
  const form = useFormContext();

  const { list: knowledgeList } = useFetchKnowledgeList(true);

  const knowledgeOptions = knowledgeList
    .filter((x) => x.parser_id === 'tag')
    .map((x) => ({
      label: x.name,
      value: x.id,
      icon: () => (
        <Space>
          <RAGFlowAvatar
            name={x.name}
            avatar={x.avatar}
            className="size-4"
          ></RAGFlowAvatar>
        </Space>
      ),
    }));

  return (
    <FormField
      control={form.control}
      name="parser_config.tag_kb_ids"
      render={({ field }) => (
        <FormItem className=" items-center space-y-0 ">
          <div className="flex items-center">
            <FormLabel
              className="text-sm text-text-secondary whitespace-nowrap w-1/4"
              tooltip={
                <div
                  dangerouslySetInnerHTML={{
                    __html: DOMPurify.sanitize(
                      t('knowledgeConfiguration.tagSetTip'),
                    ),
                  }}
                ></div>
              }
            >
              {t('knowledgeConfiguration.tagSet')}
            </FormLabel>
            <div className="w-3/4">
              <FormControl>
                <MultiSelect
                  options={knowledgeOptions}
                  onValueChange={field.onChange}
                  placeholder={t('chat.knowledgeBasesMessage')}
                  variant="inverted"
                  maxCount={10}
                  {...field}
                />
              </FormControl>
            </div>
          </div>
          <div className="flex pt-1">
            <div className="w-1/4"></div>
            <FormMessage />
          </div>
        </FormItem>
      )}
    />
  );

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
    <SliderInputFormField
      name={'parser_config.topn_tags'}
      label={t('knowledgeConfiguration.topnTags')}
      max={10}
      min={1}
      defaultValue={3}
      layout={FormLayout.Horizontal}
    ></SliderInputFormField>
  );
};

export function TagItems() {
  const form = useFormContext();
  const ids: string[] = useWatch({
    control: form.control,
    name: 'parser_config.tag_kb_ids',
  });

  return (
    <>
      <TagSetItem></TagSetItem>
      {Array.isArray(ids) && ids.length > 0 && <TopNTagsItem></TopNTagsItem>}
    </>
  );
}
