import { MinusCircleOutlined, PlusOutlined } from '@ant-design/icons';
import { Button, Collapse, Flex, Form, Input, Select } from 'antd';
import { PropsWithChildren } from 'react';
import { useTranslation } from 'react-i18next';

import styles from './index.less';

interface IProps {
  name: string;
}

const DynamicVariableForm = ({ name: formName }: IProps) => {
  const { t } = useTranslation();

  return (
    <Form.List name={formName}>
      {(fields, { add, remove }) => (
        <>
          {fields.map(({ key, name, ...restField }) => (
            <Flex key={key} gap={10} align={'baseline'}>
              <Form.Item
                {...restField}
                name={[name, 'key']}
                className={styles.variableType}
                rules={[{ required: true }]}
              >
                <Input
                  placeholder={t('setting.mcpServerVariableKey')}
                />
              </Form.Item>
              <Form.Item
                {...restField}
                name={[name, 'name']}
                className={styles.variableValue}
                rules={[{ required: true }]}
              >
                <Input
                  placeholder={t('setting.mcpServerVariableName')}
                />
              </Form.Item>
              <MinusCircleOutlined onClick={() => remove(name)} />
            </Flex>
          ))}
          <Form.Item>
            <Button
              type="dashed"
              onClick={add}
              block
              icon={<PlusOutlined />}
              className={styles.addButton}
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
      className={styles.dynamicInputVariable}
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

const McpServerVariable = () => {
  return (
    <FormCollapse title=''>
      <DynamicVariableForm name="serverVariables"></DynamicVariableForm>
    </FormCollapse>
  );
};

export default McpServerVariable;
