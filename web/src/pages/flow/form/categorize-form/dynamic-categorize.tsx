import { useTranslate } from '@/hooks/common-hooks';
import { CloseOutlined } from '@ant-design/icons';
import {
  Button,
  Card,
  Flex,
  Form,
  FormListFieldData,
  Input,
  Select,
} from 'antd';
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
import { Operator } from '../../constant';
import { useBuildFormSelectOptions } from '../../form-hooks';

import styles from './index.less';

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
  formListName: string = 'items',
  field: FormListFieldData,
  latestField: string,
) =>
  (form.getFieldValue([formListName]) ?? [])
    .map((x: any) => x[latestField])
    .filter(
      (x: string) =>
        x !== form.getFieldValue([formListName, field.name, latestField]),
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
            const idx = form.getFieldValue([
              'items',
              fields.at(-1)?.name,
              'index',
            ]);
            add({
              name: humanId(),
              index: fields.length === 0 ? 0 : idx + 1,
            });
            if (nodeId) updateNodeInternals(nodeId);
          };
          return (
            <Flex gap={18} vertical>
              {fields.map((field) => (
                <Card
                  size="small"
                  key={field.key}
                  className={styles.caseCard}
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
                      otherNames={getOtherFieldValues(
                        form,
                        'items',
                        field,
                        'name',
                      )}
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
                        getOtherFieldValues(form, 'items', field, 'to'),
                      )}
                    />
                  </Form.Item>
                  <Form.Item hidden name={[field.name, 'index']}>
                    <Input />
                  </Form.Item>
                </Card>
              ))}

              <Button
                type="dashed"
                onClick={handleAdd}
                block
                className={styles.addButton}
              >
                + {t('addItem')}
              </Button>
            </Flex>
          );
        }}
      </Form.List>
    </>
  );
};

export default DynamicCategorize;
