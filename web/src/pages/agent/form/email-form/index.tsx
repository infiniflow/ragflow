import { useTranslate } from '@/hooks/common-hooks';
import { Form, Input } from 'antd';
import { IOperatorForm } from '../../interface';
import DynamicInputVariable from '../components/dynamic-input-variable';

const EmailForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslate('flow');

  return (
    <Form
      name="basic"
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
      layout={'vertical'}
    >
      <DynamicInputVariable node={node}></DynamicInputVariable>

      {/* SMTP服务器配置 */}
      <Form.Item label={t('smtpServer')} name={'smtp_server'}>
        <Input placeholder="smtp.example.com" />
      </Form.Item>
      <Form.Item label={t('smtpPort')} name={'smtp_port'}>
        <Input type="number" placeholder="587" />
      </Form.Item>
      <Form.Item label={t('senderEmail')} name={'email'}>
        <Input placeholder="sender@example.com" />
      </Form.Item>
      <Form.Item label={t('authCode')} name={'password'}>
        <Input.Password placeholder="your_password" />
      </Form.Item>
      <Form.Item label={t('senderName')} name={'sender_name'}>
        <Input placeholder="Sender Name" />
      </Form.Item>

      {/* 动态参数说明 */}
      <div style={{ marginBottom: 24 }}>
        <h4>{t('dynamicParameters')}</h4>
        <div>{t('jsonFormatTip')}</div>
        <pre style={{ background: '#f5f5f5', padding: 12, borderRadius: 4 }}>
          {`{
  "to_email": "recipient@example.com",  
  "cc_email": "cc@example.com",
  "subject": "Email Subject",           
  "content": "Email Content"            
}`}
        </pre>
      </div>
    </Form>
  );
};

export default EmailForm;
