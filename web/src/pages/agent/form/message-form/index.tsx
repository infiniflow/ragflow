import { useTranslate } from '@/hooks/common-hooks';
import { MinusCircleOutlined, PlusOutlined } from '@ant-design/icons';
import { Button, Form, Input } from 'antd';
import { IOperatorForm } from '../../interface';

import styles from './index.less';

const formItemLayout = {
  labelCol: {
    sm: { span: 6 },
  },
  wrapperCol: {
    sm: { span: 18 },
  },
};

const formItemLayoutWithOutLabel = {
  wrapperCol: {
    sm: { span: 18, offset: 6 },
  },
};

const MessageForm = ({ onValuesChange, form }: IOperatorForm) => {
  const { t } = useTranslate('flow');

  return (
    <Form
      name="basic"
      {...formItemLayoutWithOutLabel}
      onValuesChange={onValuesChange}
      autoComplete="off"
      form={form}
    >
      <Form.List name="messages">
        {(fields, { add, remove }, {}) => (
          <>
            {fields.map((field, index) => (
              <Form.Item
                {...(index === 0 ? formItemLayout : formItemLayoutWithOutLabel)}
                label={index === 0 ? t('msg') : ''}
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
                      message: t('messageMsg'),
                    },
                  ]}
                  noStyle
                >
                  <Input.TextArea
                    rows={4}
                    placeholder={t('messagePlaceholder')}
                    style={{ width: '80%' }}
                  />
                </Form.Item>
                {fields.length > 1 ? (
                  <MinusCircleOutlined
                    className={styles.dynamicDeleteButton}
                    onClick={() => remove(field.name)}
                  />
                ) : null}
              </Form.Item>
            ))}
            <Form.Item>
              <Button
                type="dashed"
                onClick={() => add()}
                style={{ width: '80%' }}
                icon={<PlusOutlined />}
              >
                {t('addMessage')}
              </Button>
            </Form.Item>
          </>
        )}
      </Form.List>
    </Form>
  );
};

export default MessageForm;
