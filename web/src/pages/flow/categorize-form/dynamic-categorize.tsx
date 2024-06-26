import { CloseOutlined } from '@ant-design/icons';
import { Button, Card, Form, Input, Select, Typography } from 'antd';
import { useBuildCategorizeToOptions, useHandleToSelectChange } from './hooks';

interface IProps {
  nodeId?: string;
}

const DynamicCategorize = ({ nodeId }: IProps) => {
  const form = Form.useFormInstance();
  const options = useBuildCategorizeToOptions();
  const { handleSelectChange } = useHandleToSelectChange(
    options.map((x) => x.value),
    nodeId,
  );

  return (
    <>
      <Form.List name="items">
        {(fields, { add, remove }) => {
          const handleAdd = () => {
            const idx = fields.length;
            add({ name: `Categorize ${idx + 1}` });
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
                    label="name"
                    name={[field.name, 'name']}
                    // initialValue={`Categorize ${field.name + 1}`}
                    rules={[
                      { required: true, message: 'Please input your name!' },
                    ]}
                  >
                    <Input />
                  </Form.Item>
                  <Form.Item
                    label="description"
                    name={[field.name, 'description']}
                  >
                    <Input.TextArea rows={3} />
                  </Form.Item>
                  <Form.Item label="examples" name={[field.name, 'examples']}>
                    <Input.TextArea rows={3} />
                  </Form.Item>
                  <Form.Item label="to" name={[field.name, 'to']}>
                    <Select
                      allowClear
                      options={options}
                      onChange={handleSelectChange}
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

export default DynamicCategorize;
