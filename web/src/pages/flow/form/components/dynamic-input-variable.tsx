import { MinusCircleOutlined, PlusOutlined } from '@ant-design/icons';
import { Button, Collapse, Flex, Form, Input, Select } from 'antd';

import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useBuildComponentIdSelectOptions } from '../../hooks';
import styles from './index.less';

interface IProps {
  nodeId?: string;
}

enum VariableType {
  Reference = 'reference',
  Input = 'input',
}

const getVariableName = (type: string) =>
  type === VariableType.Reference ? 'component_id' : 'value';

const DynamicVariableForm = ({ nodeId }: IProps) => {
  const { t } = useTranslation();
  const valueOptions = useBuildComponentIdSelectOptions(nodeId);
  const form = Form.useFormInstance();

  const options = [
    { value: VariableType.Reference, label: t('flow.reference') },
    { value: VariableType.Input, label: t('flow.input') },
  ];

  const handleTypeChange = useCallback(
    (name: number) => () => {
      setTimeout(() => {
        form.setFieldValue(['query', name, 'component_id'], undefined);
        form.setFieldValue(['query', name, 'value'], undefined);
      }, 0);
    },
    [form],
  );

  return (
    <Form.List name="query">
      {(fields, { add, remove }) => (
        <>
          {fields.map(({ key, name, ...restField }) => (
            <Flex key={key} gap={10} align={'baseline'}>
              <Form.Item
                {...restField}
                name={[name, 'type']}
                className={styles.variableType}
              >
                <Select
                  options={options}
                  onChange={handleTypeChange(name)}
                ></Select>
              </Form.Item>
              <Form.Item noStyle dependencies={[name, 'type']}>
                {({ getFieldValue }) => {
                  const type = getFieldValue(['query', name, 'type']);
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
            >
              {t('flow.addItem')}
            </Button>
          </Form.Item>
        </>
      )}
    </Form.List>
  );
};

const DynamicInputVariable = ({ nodeId }: IProps) => {
  const { t } = useTranslation();

  return (
    <Collapse
      className={styles.dynamicInputVariable}
      defaultActiveKey={['1']}
      items={[
        {
          key: '1',
          label: <span className={styles.title}>{t('flow.input')}</span>,
          children: <DynamicVariableForm nodeId={nodeId}></DynamicVariableForm>,
        },
      ]}
    />
  );
};

export default DynamicInputVariable;
