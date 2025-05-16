import { MinusCircleOutlined, PlusOutlined } from '@ant-design/icons';
import { Button, Form, Input, Select } from 'antd';
import { useTranslation } from 'react-i18next';
import { FormCollapse } from '../components/dynamic-input-variable';

type DynamicOutputVariableProps = {
  name?: string;
};

const options = [
  'String',
  'Number',
  'Boolean',
  'Array[String]',
  'Array[Number]',
  'Object',
].map((x) => ({ label: x, value: x }));

export const DynamicOutputVariable = ({
  name = 'output',
}: DynamicOutputVariableProps) => {
  const { t } = useTranslation();

  return (
    <FormCollapse title={t('flow.output')}>
      <Form.List name={name}>
        {(fields, { add, remove }) => (
          <>
            {fields.map(({ key, name, ...restField }) => (
              <div key={key} className="flex items-center gap-2 pb-4">
                <Form.Item
                  {...restField}
                  name={[name, 'first']}
                  className="m-0 flex-1"
                >
                  <Input />
                </Form.Item>
                <Form.Item
                  {...restField}
                  name={[name, 'last']}
                  className="m-0 flex-1"
                >
                  <Select
                    placeholder={t('common.pleaseSelect')}
                    options={options}
                  ></Select>
                </Form.Item>
                <MinusCircleOutlined onClick={() => remove(name)} />
              </div>
            ))}
            <Form.Item>
              <Button
                type="dashed"
                onClick={() => add()}
                block
                icon={<PlusOutlined />}
              >
                {t('flow.addVariable')}
              </Button>
            </Form.Item>
          </>
        )}
      </Form.List>
    </FormCollapse>
  );
};
