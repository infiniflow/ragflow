import CopyToClipboard from '@/components/copy-to-clipboard';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { t } from 'i18next';
import { useFormContext } from 'react-hook-form';

interface IApiKeyFieldProps {
  placeholder?: string;
}
export function ApiKeyField({ placeholder }: IApiKeyFieldProps) {
  const form = useFormContext();
  return (
    <FormField
      control={form.control}
      name="api_key"
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t('flow.apiKey')}</FormLabel>
          <FormControl>
            <Input
              type="password"
              {...field}
              placeholder={placeholder}
              suffix={
                field.value ? (
                  <CopyToClipboard
                    text={String(field.value)}
                    type="button"
                    tabIndex={-1}
                  />
                ) : null
              }
            ></Input>
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
