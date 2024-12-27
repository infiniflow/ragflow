import { MinusCircleOutlined, PlusOutlined } from '@ant-design/icons';
import { Button, Form, Input } from 'antd';

const BeginDynamicOptions = () => {
  return (
    <Form.List
      name="options"
      rules={[
        {
          validator: async (_, names) => {
            if (!names || names.length < 1) {
              return Promise.reject(new Error('At least 1 option'));
            }
          },
        },
      ]}
    >
      {(fields, { add, remove }, { errors }) => (
        <>
          {fields.map((field, index) => (
            <Form.Item
              label={index === 0 ? 'Options' : ''}
              required={false}
              key={field.key}
            >
              <Form.Item
                {...field}
                validateTrigger={['onChange', 'onBlur']}
                rules={[
                  {
                    required: true,
                    whitespace: true,
                    message: 'Please input option or delete this field.',
                  },
                ]}
                noStyle
              >
                <Input
                  placeholder="option"
                  style={{ width: '90%', marginRight: 16 }}
                />
              </Form.Item>
              {fields.length > 1 ? (
                <MinusCircleOutlined
                  className="dynamic-delete-button"
                  onClick={() => remove(field.name)}
                />
              ) : null}
            </Form.Item>
          ))}
          <Form.Item>
            <Button
              type="dashed"
              onClick={() => add()}
              icon={<PlusOutlined />}
              block
            >
              Add option
            </Button>
            <Form.ErrorList errors={errors} />
          </Form.Item>
        </>
      )}
    </Form.List>
  );
};

export default BeginDynamicOptions;
