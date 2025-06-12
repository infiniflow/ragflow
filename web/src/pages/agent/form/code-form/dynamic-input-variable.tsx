import { BlockButton } from '@/components/ui/button';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { MinusCircleOutlined } from '@ant-design/icons';
import { Form, Input, Select } from 'antd';
import { useTranslation } from 'react-i18next';
import { useBuildVariableOptions } from '../../hooks/use-get-begin-query';
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

  const valueOptions = useBuildVariableOptions(node?.id, node?.parentId);

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
              <BlockButton onClick={() => add()}>
                {t('flow.addVariable')}
              </BlockButton>
            </Form.Item>
          </>
        )}
      </Form.List>
    </FormCollapse>
  );
};
