import { useTranslate } from '@/hooks/commonHooks';
import { CloseOutlined } from '@ant-design/icons';
import { Button, Card, Form, Input, Select, Typography } from 'antd';
import { useUpdateNodeInternals } from 'reactflow';
import { Operator } from '../constant';
import {
  useBuildFormSelectOptions,
  useHandleFormSelectChange,
} from '../form-hooks';
import { IGenerateParameter } from '../interface';

interface IProps {
  nodeId?: string;
}

const DynamicParameters = ({ nodeId }: IProps) => {
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
      <Form.List name="parameters">
        {(fields, { add, remove }) => {
          const handleAdd = () => {
            const idx = fields.length;
            add({ name: `parameter ${idx + 1}` });
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
                    label={t('key')} // TODO: repeatability check
                    name={[field.name, 'key']}
                    rules={[{ required: true, message: t('nameMessage') }]}
                  >
                    <Input />
                  </Form.Item>
                  <Form.Item
                    label={t('componentId')}
                    name={[field.name, 'component_id']}
                  >
                    <Select
                      allowClear
                      options={buildCategorizeToOptions(
                        (form.getFieldValue(['parameters']) ?? [])
                          .map((x: IGenerateParameter) => x.component_id)
                          .filter(
                            (x: string) =>
                              x !==
                              form.getFieldValue([
                                'parameters',
                                field.name,
                                'component_id',
                              ]),
                          ),
                      )}
                      onChange={handleSelectChange(
                        form.getFieldValue(['parameters', field.name, 'key']),
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

      <Form.Item noStyle shouldUpdate>
        {() => (
          <Typography>
            <pre>{JSON.stringify(form.getFieldsValue(), null, 2)}</pre>
          </Typography>
        )}
      </Form.Item>
    </>
  );
};

export default DynamicParameters;
