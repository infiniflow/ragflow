import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import NumberInput from './originui/number-input';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from './ui/form';

type MessageHistoryWindowSizeFormFieldProps = {
  min?: number;
};
export function MessageHistoryWindowSizeFormField({
  min,
}: MessageHistoryWindowSizeFormFieldProps) {
  const form = useFormContext();
  const { t } = useTranslation();

  return (
    <FormField
      control={form.control}
      name={'message_history_window_size'}
      render={({ field }) => (
        <FormItem>
          <FormLabel tooltip={t('flow.messageHistoryWindowSizeTip')}>
            {t('flow.messageHistoryWindowSize')}
          </FormLabel>
          <FormControl>
            <NumberInput {...field} min={min} className="w-full"></NumberInput>
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
