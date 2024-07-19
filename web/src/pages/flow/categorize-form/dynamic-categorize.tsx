import { useTranslate } from '@/hooks/common-hooks';
import { CloseOutlined } from '@ant-design/icons';
import { Button, Card, Form, FormListFieldData, Input, Select } from 'antd';
import { FormInstance } from 'antd/lib';
import { humanId } from 'human-id';
import trim from 'lodash/trim';
import {
  ChangeEventHandler,
  FocusEventHandler,
  useCallback,
  useEffect,
  useState,
} from 'react';
import { useUpdateNodeInternals } from 'reactflow';
import { Operator } from '../constant';
import { useBuildFormSelectOptions } from '../form-hooks';

interface IProps {
  nodeId?: string;
}

interface INameInputProps {
  value?: string;
  onChange?: (value: string) => void;
  otherNames?: string[];
  validate(errors: string[]): void;
}

const getOtherFieldValues = (
  form: FormInstance,
  field: FormListFieldData,
  latestField: string,
) =>
  (form.getFieldValue(['items']) ?? [])
    .map((x: any) => x[latestField])
    .filter(
      (x: string) =>
        x !== form.getFieldValue(['items', field.name, latestField]),
    );

const NameInput = ({
  value,
  onChange,
  otherNames,
  validate,
}: INameInputProps) => {
  const [name, setName] = useState<string | undefined>();
  const { t } = useTranslate('flow');

  const handleNameChange: ChangeEventHandler<HTMLInputElement> = useCallback(
    (e) => {
      const val = e.target.value;
      // trigger validation
      if (otherNames?.some((x) => x === val)) {
        validate([t('nameRepeatedMsg')]);
      } else if (trim(val) === '') {
        validate([t('nameRequiredMsg')]);
      } else {
        validate([]);
      }
      setName(val);
    },
    [otherNames, validate, t],
  );

  const handleNameBlur: FocusEventHandler<HTMLInputElement> = useCallback(
    (e) => {
      const val = e.target.value;
      if (otherNames?.every((x) => x !== val) && trim(val) !== '') {
        onChange?.(val);
      }
    },
    [onChange, otherNames],
  );

  useEffect(() => {
    setName(value);
  }, [value]);

  return (
    <Input
      value={name}
      onChange={handleNameChange}
      onBlur={handleNameBlur}
    ></Input>
  );
};

const DynamicCategorize = ({ nodeId }: IProps) => {
  const updateNodeInternals = useUpdateNodeInternals();
  const form = Form.useFormInstance();
  const buildCategorizeToOptions = useBuildFormSelectOptions(
    Operator.Categorize,
    nodeId,
  );
  const { t } = useTranslate('flow');

  return (
    <>
      <Form.List name="items">
        {(fields, { add, remove }) => {
          const handleAdd = () => {
            add({ name: humanId() });
            if (nodeId) updateNodeInternals(nodeId);
          };
          return (
            <div
              style={{ display: 'flex', rowGap: 10, flexDirection: 'column' }}
            >
              {fields.map((field) => (
                <Card
                  size="small"
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
                    label={t('name')}
                    name={[field.name, 'name']}
                    validateTrigger={['onChange', 'onBlur']}
                    rules={[
                      {
                        required: true,
                        whitespace: true,
                        message: t('nameMessage'),
                      },
                    ]}
                  >
                    <NameInput
                      otherNames={getOtherFieldValues(form, field, 'name')}
                      validate={(errors: string[]) =>
                        form.setFields([
                          {
                            name: ['items', field.name, 'name'],
                            errors,
                          },
                        ])
                      }
                    ></NameInput>
                  </Form.Item>
                  <Form.Item
                    label={t('description')}
                    name={[field.name, 'description']}
                  >
                    <Input.TextArea rows={3} />
                  </Form.Item>
                  <Form.Item
                    label={t('examples')}
                    name={[field.name, 'examples']}
                  >
                    <Input.TextArea rows={3} />
                  </Form.Item>
                  <Form.Item label={t('to')} name={[field.name, 'to']}>
                    <Select
                      allowClear
                      options={buildCategorizeToOptions(
                        getOtherFieldValues(form, field, 'to'),
                      )}
                    />
                  </Form.Item>
                </Card>
              ))}

              <Button type="dashed" onClick={handleAdd} block>
                + {t('addItem')}
              </Button>
            </div>
          );
        }}
      </Form.List>

      {/* <Form.Item noStyle shouldUpdate>
        {() => (
          <Typography>
            <pre>{JSON.stringify(form.getFieldsValue(), null, 2)}</pre>
          </Typography>
        )}
      </Form.Item> */}
    </>
  );
};

export default DynamicCategorize;
