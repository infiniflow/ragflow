import { CloseOutlined } from '@ant-design/icons';
import { Button, Card, Form, Input, Select, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { Operator, SwitchElseTo } from '../constant';
import { useBuildFormSelectOptions } from '../form-hooks';
import { IOperatorForm, ISwitchForm } from '../interface';
import { getOtherFieldValues } from '../utils';

const subLabelCol = {
  span: 7,
};

const subWrapperCol = {
  span: 17,
};

const SwitchForm = ({ onValuesChange, node, form }: IOperatorForm) => {
  const { t } = useTranslation();
  const buildCategorizeToOptions = useBuildFormSelectOptions(
    Operator.Switch,
    node?.id,
  );

  const getSelectedConditionTos = () => {
    const conditions: ISwitchForm['conditions'] =
      form?.getFieldValue('conditions');

    return conditions?.filter((x) => !!x).map((x) => x?.to) ?? [];
  };

  return (
    <Form
      labelCol={{ span: 8 }}
      wrapperCol={{ span: 16 }}
      form={form}
      name="dynamic_form_complex"
      autoComplete="off"
      initialValues={{ conditions: [{}] }}
      onValuesChange={onValuesChange}
    >
      <Form.Item label={t('flow.to')} name={[SwitchElseTo]}>
        <Select
          allowClear
          options={buildCategorizeToOptions(getSelectedConditionTos())}
        />
      </Form.Item>
      <Form.List name="conditions">
        {(fields, { add, remove }) => (
          <div style={{ display: 'flex', rowGap: 16, flexDirection: 'column' }}>
            {fields.map((field) => (
              <Card
                size="small"
                title={`Item ${field.name + 1}`}
                key={field.key}
                extra={
                  <CloseOutlined
                    onClick={() => {
                      remove(field.name);
                    }}
                  />
                }
              >
                <Form.Item
                  label={t('flow.logicalOperator')}
                  name={[field.name, 'logical_operator']}
                >
                  <Input />
                </Form.Item>

                <Form.Item label={t('flow.to')} name={[field.name, 'to']}>
                  <Select
                    allowClear
                    options={buildCategorizeToOptions([
                      form?.getFieldValue(SwitchElseTo),
                      ...getOtherFieldValues(form!, 'conditions', field, 'to'),
                    ])}
                  />
                </Form.Item>
                <Form.Item label="Items">
                  <Form.List name={[field.name, 'items']}>
                    {(subFields, subOpt) => (
                      <div
                        style={{
                          display: 'flex',
                          flexDirection: 'column',
                          rowGap: 16,
                        }}
                      >
                        {subFields.map((subField) => (
                          <Card
                            key={subField.key}
                            title={null}
                            size="small"
                            extra={
                              <CloseOutlined
                                onClick={() => {
                                  subOpt.remove(subField.name);
                                }}
                              />
                            }
                          >
                            <Form.Item
                              label={'cpn_id'}
                              name={[subField.name, 'cpn_id']}
                              labelCol={subLabelCol}
                              wrapperCol={subWrapperCol}
                            >
                              <Input placeholder="cpn_id" />
                            </Form.Item>
                            <Form.Item
                              label={'operator'}
                              name={[subField.name, 'operator']}
                              labelCol={subLabelCol}
                              wrapperCol={subWrapperCol}
                            >
                              <Input placeholder="operator" />
                            </Form.Item>
                            <Form.Item
                              label={'value'}
                              name={[subField.name, 'value']}
                              labelCol={subLabelCol}
                              wrapperCol={subWrapperCol}
                            >
                              <Input placeholder="value" />
                            </Form.Item>
                          </Card>
                        ))}
                        <Button
                          type="dashed"
                          onClick={() => subOpt.add()}
                          block
                        >
                          + Add Sub Item
                        </Button>
                      </div>
                    )}
                  </Form.List>
                </Form.Item>
              </Card>
            ))}

            <Button type="dashed" onClick={() => add()} block>
              + Add Item
            </Button>
          </div>
        )}
      </Form.List>

      <Form.Item noStyle shouldUpdate>
        {() => (
          <Typography>
            <pre>{JSON.stringify(form?.getFieldsValue(), null, 2)}</pre>
          </Typography>
        )}
      </Form.Item>
    </Form>
  );
};

export default SwitchForm;
