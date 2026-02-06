import { useTranslate } from '@/hooks/common-hooks';
import { useFormContext } from 'react-hook-form';
import PasswordInput from './originui/password-input';
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from './ui/form';

interface IProps {
  name?: string;
}

export function TavilyFormField({
  name = 'prompt_config.tavily_api_key',
}: IProps) {
  const form = useFormContext();
  const { t } = useTranslate('chat');

  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel tooltip={t('tavilyApiKeyTip')}>Tavily API Key</FormLabel>
          <FormControl>
            <PasswordInput
              {...field}
              placeholder={t('tavilyApiKeyMessage')}
              autoComplete="new-password"
            ></PasswordInput>
          </FormControl>
          <FormDescription>
            <a
              href="https://app.tavily.com/home"
              target={'_blank'}
              rel="noreferrer"
            >
              {t('tavilyApiKeyHelp')}
            </a>
          </FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
