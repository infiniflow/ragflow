import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { MinusCircleOutlined, PlusOutlined } from '@ant-design/icons';
import { Button, Form, Input, Select } from 'antd';
import { useTranslation } from 'react-i18next';
import { useBuildComponentIdSelectOptions } from '../../hooks/use-get-begin-query';
import { FormCollapse } from '../components/dynamic-input-variable';

type DynamicInputVariableProps = {
  name?: string;
  node?: RAGFlowNodeType;
};

export const DynamicInputVariable = ({
  name = 'arguments',
  node,
}: DynamicInputVariableProps) => {
  const { t } = useTranslation();

  const valueOptions = useBuildComponentIdSelectOptions(
    node?.id,
    node?.parentId,
  );

  return (
    <FormCollapse title={t('flow.inputVariables')}>
      <Form.List name={name}>
        {(fields, { add, remove }) => (
          <>
            {fields.map(({ key, name, ...restField }) => (
              <div key={key} className="flex items-center gap-2 pb-4">
                <Form.Item
                  {...restField}
                  name={[name, 'name']}
                  className="m-0 flex-1"
                >
                  <Input />
                </Form.Item>
                <Form.Item
                  {...restField}
                  name={[name, 'component_id']}
                  className="m-0 flex-1"
                >
                  <Select
                    placeholder={t('common.pleaseSelect')}
                    options={valueOptions}
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
