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
      <DynamicInputVariable nodeId={node?.id}></DynamicInputVariable>

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
          {`
{
  "to_email": "recipient@example.com",  
  "cc_email": "cc@example.com",
  "subject": "Email Subject",           
  "content": "Email Content"            
}
`}
        </pre>
        {'发送成功返回True'}
        {'报错信息'}
        {`
101： 输入的JSON格式无效
102： SMTP认证失败。请检查您的邮箱和授权码。
103： 无法连接到SMTP服务器
104： 发生SMTP错误
105： 发生意外错误
`}
      </div>
    </Form>
  );
};

export default EmailForm;
