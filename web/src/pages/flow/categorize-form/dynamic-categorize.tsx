import { useTranslate } from '@/hooks/commonHooks';
import { CloseOutlined } from '@ant-design/icons';
import { Button, Card, Form, Input, Select } from 'antd';
import { useUpdateNodeInternals } from 'reactflow';
import { Operator } from '../constant';
import {
  useBuildFormSelectOptions,
  useHandleFormSelectChange,
} from '../form-hooks';
import { ICategorizeItem } from '../interface';

interface IProps {
  nodeId?: string;
}

const DynamicCategorize = ({ nodeId }: IProps) => {
  const updateNodeInternals = useUpdateNodeInternals();
  const form = Form.useFormInstance();
  const buildCategorizeToOptions = useBuildFormSelectOptions(
    Operator.Categorize,
    nodeId,
  );
  const { handleSelectChange } = useHandleFormSelectChange(nodeId);
  const { t } = useTranslate('flow');

  return (
    <>
      <Form.List name="items">
        {(fields, { add, remove }) => {
          const handleAdd = () => {
            const idx = fields.length;
            add({ name: `Categorize ${idx + 1}` });
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
                    label={t('name')} // TODO: repeatability check
                    name={[field.name, 'name']}
                    rules={[{ required: true, message: t('nameMessage') }]}
                  >
                    <Input />
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
                        (form.getFieldValue(['items']) ?? [])
                          .map((x: ICategorizeItem) => x.to)
                          .filter(
                            (x: string) =>
                              x !==
                              form.getFieldValue(['items', field.name, 'to']),
                          ),
                      )}
                      onChange={handleSelectChange(
                        form.getFieldValue(['items', field.name, 'name']),
                      )}
                    />
                  </Form.Item>
                </Card>
              ))}

              <Button type="dashed" onClick={handleAdd} block>
                + Add Item
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
