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

/**
 * Form field for entering an `api_key`. Renders a password-masked input so
 * the value is not visible on screen, with a clipboard copy button in the
 * input's suffix so users can retrieve a previously-saved key without
 * unmasking it. Must be rendered inside a `react-hook-form` `FormProvider`
 * — reads and writes the `api_key` field via `useFormContext`.
 *
 * @param placeholder Optional placeholder shown when the field is empty.
 */
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
                  <CopyToClipboard text={String(field.value)} type="button" />
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
