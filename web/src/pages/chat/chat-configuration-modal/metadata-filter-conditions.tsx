import { SwitchOperatorOptions } from '@/constants/agent';
import { useBuildSwitchOperatorOptions } from '@/hooks/logic-hooks/use-build-operator-options';
import { useFetchKnowledgeMetadata } from '@/hooks/use-knowledge-request';
import { MinusCircleOutlined, PlusOutlined } from '@ant-design/icons';
import {
  Button,
  Dropdown,
  Empty,
  Form,
  FormListOperation,
  Input,
  Select,
  Space,
} from 'antd';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

export function MetadataFilterConditions({ kbIds }: { kbIds: string[] }) {
  const metadata = useFetchKnowledgeMetadata(kbIds);
  const { t } = useTranslation();
  const switchOperatorOptions = useBuildSwitchOperatorOptions();

  const renderItems = useCallback(
    (add: FormListOperation['add']) => {
      if (Object.keys(metadata.data).length === 0) {
        return [{ key: 'noData', label: <Empty></Empty> }];
      }
      return Object.keys(metadata.data).map((key) => {
        return {
          key,
          onClick: () => {
            add({
              key,
              value: '',
              op: SwitchOperatorOptions[0].value,
            });
          },
          label: key,
        };
      });
    },
    [metadata],
  );
  return (
    <Form.List name={['meta_data_filter', 'manual']}>
      {(fields, { add, remove }) => (
        <>
          {fields.map(({ key, name, ...restField }) => (
            <Space
              key={key}
              style={{ display: 'flex', marginBottom: 8 }}
              align="baseline"
            >
              <Form.Item
                {...restField}
                name={[name, 'key']}
                rules={[{ required: true, message: t('common.pleaseInput') }]}
              >
                <Input placeholder={t('common.pleaseInput')} />
              </Form.Item>
              <Form.Item {...restField} name={[name, 'op']} className="w-20">
                <Select
                  options={switchOperatorOptions}
                  popupMatchSelectWidth={false}
                />
              </Form.Item>
              <Form.Item
                {...restField}
                name={[name, 'value']}
                rules={[{ required: true, message: t('common.pleaseInput') }]}
              >
                <Input placeholder={t('common.pleaseInput')} />
              </Form.Item>
              <MinusCircleOutlined onClick={() => remove(name)} />
            </Space>
          ))}
          <Form.Item>
            <Dropdown trigger={['click']} menu={{ items: renderItems(add) }}>
              <Button type="dashed" block icon={<PlusOutlined />}>
                {t('chat.addCondition')}
              </Button>
            </Dropdown>
          </Form.Item>
        </>
      )}
    </Form.List>
  );
}
