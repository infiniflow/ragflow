import { CloseOutlined } from '@ant-design/icons';
import { Button, Card, Divider, Form, Input, Select } from 'antd';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Operator,
  SwitchElseTo,
  SwitchLogicOperatorOptions,
  SwitchOperatorOptions,
} from '../../constant';
import { useBuildFormSelectOptions } from '../../form-hooks';
import { useBuildComponentIdSelectOptions } from '../../hooks';
import { IOperatorForm, ISwitchForm } from '../../interface';
import { getOtherFieldValues } from '../../utils';

const subLabelCol = {
  span: 11,
};

const subWrapperCol = {
  span: 13,
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

  const switchOperatorOptions = useMemo(() => {
    return SwitchOperatorOptions.map((x) => ({
      value: x.value,
      label: t(`flow.switchOperatorOptions.${x.label}`),
    }));
  }, [t]);

  const switchLogicOperatorOptions = useMemo(() => {
    return SwitchLogicOperatorOptions.map((x) => ({
      value: x,
      label: t(`flow.switchLogicOperatorOptions.${x}`),
    }));
  }, [t]);

  const componentIdOptions = useBuildComponentIdSelectOptions(node?.id);

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
      <Form.List name="conditions">
        {(fields, { add, remove }) => (
          <div style={{ display: 'flex', rowGap: 16, flexDirection: 'column' }}>
            {fields.map((field) => (
              <Card
                size="small"
                title={`Case ${field.name + 1}`}
                key={field.key}
                extra={
                  <CloseOutlined
                    onClick={() => {
                      remove(field.name);
                    }}
                  />
                }
              >
                <Form.Item noStyle dependencies={[field.name, 'items']}>
                  {({ getFieldValue }) =>
                    getFieldValue(['conditions', field.name, 'items'])?.length >
                      1 && (
                      <Form.Item
                        label={t('flow.logicalOperator')}
                        name={[field.name, 'logical_operator']}
                      >
                        <Select options={switchLogicOperatorOptions} />
                      </Form.Item>
                    )
                  }
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
                <Form.Item label="Condition">
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
                              label={t('flow.componentId')}
                              name={[subField.name, 'cpn_id']}
                              labelCol={subLabelCol}
                              wrapperCol={subWrapperCol}
                            >
                              <Select
                                placeholder={t('flow.componentId')}
                                options={componentIdOptions}
                              />
                            </Form.Item>
                            <Form.Item
                              label={t('flow.operator')}
                              name={[subField.name, 'operator']}
                              labelCol={subLabelCol}
                              wrapperCol={subWrapperCol}
                            >
                              <Select
                                placeholder={t('flow.operator')}
                                options={switchOperatorOptions}
                              />
                            </Form.Item>
                            <Form.Item
                              label={t('flow.value')}
                              name={[subField.name, 'value']}
                              labelCol={subLabelCol}
                              wrapperCol={subWrapperCol}
                            >
                              <Input placeholder={t('flow.value')} />
                            </Form.Item>
                          </Card>
                        ))}
                        <Button
                          type="dashed"
                          onClick={() => subOpt.add()}
                          block
                        >
                          + Add Condition
                        </Button>
                      </div>
                    )}
                  </Form.List>
                </Form.Item>
              </Card>
            ))}

            <Button type="dashed" onClick={() => add()} block>
              + Add Case
            </Button>
          </div>
        )}
      </Form.List>
      <Divider />
      <Form.Item label={'ELSE'} name={[SwitchElseTo]}>
        <Select
          allowClear
          options={buildCategorizeToOptions(getSelectedConditionTos())}
        />
      </Form.Item>
    </Form>
  );
};

export default SwitchForm;
