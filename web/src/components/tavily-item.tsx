import { useTranslate } from '@/hooks/common-hooks';
import { Form, Input, Typography } from 'antd';

interface IProps {
  name?: string | string[];
}

export function TavilyItem({
  name = ['prompt_config', 'tavily_api_key'],
}: IProps) {
  const { t } = useTranslate('chat');

  return (
    <Form.Item label={'Tavily API Key'} tooltip={t('tavilyApiKeyTip')}>
      <div className="flex flex-col gap-1">
        <Form.Item name={name} noStyle>
          <Input.Password
            placeholder={t('tavilyApiKeyMessage')}
            autoComplete="new-password"
          />
        </Form.Item>
        <Typography.Link href="https://app.tavily.com/home" target={'_blank'}>
          {t('tavilyApiKeyHelp')}
        </Typography.Link>
      </div>
    </Form.Item>
  );
}
