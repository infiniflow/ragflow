import {
  useFetchKnowledgeBaseConfiguration,
  useFetchTagListByKnowledgeIds,
} from '@/hooks/knowledge-hooks';
import { MinusCircleOutlined, PlusOutlined } from '@ant-design/icons';
import { Button, Form, InputNumber, Select } from 'antd';
import { useCallback, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { FormListItem } from '../../utils';

const FieldKey = 'tag_feas';

export const TagFeatureItem = () => {
  const form = Form.useFormInstance();
  const { t } = useTranslation();
  const { data: knowledgeConfiguration } = useFetchKnowledgeBaseConfiguration();

  const { setKnowledgeIds, list } = useFetchTagListByKnowledgeIds();

  const tagKnowledgeIds = useMemo(() => {
    return knowledgeConfiguration?.parser_config?.tag_kb_ids ?? [];
  }, [knowledgeConfiguration?.parser_config?.tag_kb_ids]);

  const options = useMemo(() => {
    return list.map((x) => ({
      value: x[0],
      label: x[0],
    }));
  }, [list]);

  const filterOptions = useCallback(
    (index: number) => {
      const tags: FormListItem[] = form.getFieldValue(FieldKey) ?? [];

      // Exclude it's own current data
      const list = tags
        .filter((x, idx) => x && index !== idx)
        .map((x) => x.tag);

      // Exclude the selected data from other options from one's own options.
      return options.filter((x) => !list.some((y) => x.value === y));
    },
    [form, options],
  );

  useEffect(() => {
    setKnowledgeIds(tagKnowledgeIds);
  }, [setKnowledgeIds, tagKnowledgeIds]);

  return (
    <Form.Item label={t('knowledgeConfiguration.tags')}>
      <Form.List name={FieldKey} initialValue={[]}>
        {(fields, { add, remove }) => (
          <>
            {fields.map(({ key, name, ...restField }) => (
              <div key={key} className="flex gap-3 items-center">
                <div className="flex flex-1  gap-8">
                  <Form.Item
                    {...restField}
                    name={[name, 'tag']}
                    rules={[
                      { required: true, message: t('common.pleaseSelect') },
                    ]}
                    className="w-2/3"
                  >
                    <Select
                      showSearch
                      placeholder={t('knowledgeConfiguration.tagName')}
                      options={filterOptions(name)}
                    />
                  </Form.Item>
                  <Form.Item
                    {...restField}
                    name={[name, 'frequency']}
                    rules={[
                      { required: true, message: t('common.pleaseInput') },
                    ]}
                  >
                    <InputNumber
                      placeholder={t('knowledgeConfiguration.frequency')}
                      max={10}
                      min={0}
                    />
                  </Form.Item>
                </div>
                <MinusCircleOutlined
                  onClick={() => remove(name)}
                  className="mb-6"
                />
              </div>
            ))}
            <Form.Item>
              <Button
                type="dashed"
                onClick={() => add()}
                block
                icon={<PlusOutlined />}
              >
                {t('knowledgeConfiguration.addTag')}
              </Button>
            </Form.Item>
          </>
        )}
      </Form.List>
    </Form.Item>
  );
};
