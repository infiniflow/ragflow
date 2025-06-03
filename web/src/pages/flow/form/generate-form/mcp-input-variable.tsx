import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { MinusCircleOutlined, PlusOutlined } from '@ant-design/icons';
import { Button, Collapse, Flex, Form, Input, Select } from 'antd';
import { PropsWithChildren, useCallback, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useBuildComponentIdSelectOptions } from '../../hooks/use-get-begin-query';

import styles from './index.less';
import { useBuildMcpServerVariableOptions } from './hooks';

interface IProps {
  name: string;
  node: RAGFlowNodeType;
  disabled?: boolean;
  newMap?: any[];
}

enum VariableType {
  Reference = 'reference',
  Input = 'input',
}

const getVariableName = (type: string) =>
  type === VariableType.Reference ? 'component_id' : 'value';

const McpVariableForm = ({ name: formName, node, disabled }: IProps) => {
  const { t } = useTranslation();

  const targetOptions = useBuildMcpServerVariableOptions(node.id);

  const valueOptions = useBuildComponentIdSelectOptions(
    node?.id,
    node?.parentId,
  );

  const form = Form.useFormInstance();

  const typeOptions = [
    { value: VariableType.Reference, label: t('flow.reference') },
    { value: VariableType.Input, label: t('flow.text') },
  ];

  const handleTypeChange = useCallback(
    (name: number) => () => {
      setTimeout(() => {
        form.setFieldValue([formName, name, 'component_id'], undefined);
        form.setFieldValue([formName, name, 'value'], undefined);
      }, 0);
    },
    [form, formName],
  );

  return (
    <Form.List name={formName}>
      {(fields, { add, remove }) => (
        <>
          {fields.map(({ key, name, ...restField }) => (
            <Flex key={key} gap={10} align={'baseline'}>
              <Form.Item
                {...restField}
                name={[name, 'target']}
                className={styles.variableTarget}
              >
                <Select
                  placeholder={t('common.pleaseSelect')}
                  options={targetOptions}
                  onChange={handleTypeChange(name)}
                  optionLabelProp="fullLabel"
                ></Select>
              </Form.Item>
              <Form.Item
                {...restField}
                name={[name, 'type']}
                className={styles.variableType}
              >
                <Select
                  options={typeOptions}
                  onChange={handleTypeChange(name)}
                ></Select>
              </Form.Item>
              <Form.Item noStyle dependencies={[name, 'type']}>
                {({ getFieldValue }) => {
                  const type = getFieldValue([formName, name, 'type']);
                  return (
                    <Form.Item
                      {...restField}
                      name={[name, getVariableName(type)]}
                      className={styles.variableValue}
                    >
                      {type === VariableType.Reference ? (
                        <Select
                          placeholder={t('common.pleaseSelect')}
                          options={valueOptions}
                        ></Select>
                      ) : (
                        <Input placeholder={t('common.pleaseInput')} />
                      )}
                    </Form.Item>
                  );
                }}
              </Form.Item>
              <MinusCircleOutlined onClick={() => remove(name)} />
            </Flex>
          ))}
          <Form.Item>
            <Button
              type="dashed"
              onClick={() => add({ type: VariableType.Reference })}
              block
              icon={<PlusOutlined />}
              className={styles.addButton}
              disabled={disabled}
            >
              {t('flow.addVariable')}
            </Button>
          </Form.Item>
        </>
      )}
    </Form.List>
  );
};

export function FormCollapse({
  children,
  title,
}: PropsWithChildren<{ title: string }>) {
  return (
    <Collapse
      className={styles.mcpInputVariable}
      defaultActiveKey={['1']}
      items={[
        {
          key: '1',
          label: <span className={styles.title}>{title}</span>,
          children,
        },
      ]}
    />
  );
}

const McpInputVariable = ({ name, node, disabled, newMap }: IProps) => {
  const { t } = useTranslation();

  useEffect(() => {
    node!!.data.form.mcp_server_variable_map = newMap;
  }, [newMap]);

  return (
    <FormCollapse title={t('flow.mcpInputVariable')}>
      <McpVariableForm
        name={name}
        node={node}
        disabled={disabled}
      ></McpVariableForm>
    </FormCollapse>
  );
};

export default McpInputVariable;
