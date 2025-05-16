import { SliderInputFormField } from '@/components/slider-input-form-field';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { MultiSelect } from '@/components/ui/multi-select';
import { useFetchKnowledgeList } from '@/hooks/knowledge-hooks';
import { UserOutlined } from '@ant-design/icons';
import { Avatar, Flex, Form, InputNumber, Select, Slider, Space } from 'antd';
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
          <Avatar size={20} icon={<UserOutlined />} src={x.avatar} />
          {x.name}
        </Space>
      ),
    }));

  return (
    <FormField
      control={form.control}
      name="parser_config.tag_kb_ids"
      render={({ field }) => (
        <FormItem>
          <FormLabel
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
          <FormControl>
            <MultiSelect
              options={knowledgeOptions}
              onValueChange={field.onChange}
              placeholder={t('chat.knowledgeBasesMessage')}
              variant="inverted"
              maxCount={0}
              {...field}
            />
          </FormControl>
          <FormMessage />
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
    ></SliderInputFormField>
  );

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
