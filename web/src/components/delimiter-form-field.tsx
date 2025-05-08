import { forwardRef } from 'react';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from './ui/form';
import { Input, InputProps } from './ui/input';

interface IProps {
  value?: string | undefined;
  onChange?: (val: string | undefined) => void;
}

export const DelimiterInput = forwardRef<HTMLInputElement, InputProps & IProps>(
  ({ value, onChange, maxLength, defaultValue }, ref) => {
    const nextValue = value?.replaceAll('\n', '\\n');
    const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
      const val = e.target.value;
      const nextValue = val.replaceAll('\\n', '\n');
      onChange?.(nextValue);
    };
    return (
      <Input
        value={nextValue}
        onChange={handleInputChange}
        maxLength={maxLength}
        defaultValue={defaultValue}
        ref={ref}
      ></Input>
    );
  },
);

export function DelimiterFormField() {
  const { t } = useTranslation();
  const form = useFormContext();

  return (
    <FormField
      control={form.control}
      name={'parser_config.delimiter'}
      render={({ field }) => (
        <FormItem>
          <FormLabel tooltip={t('knowledgeDetails.delimiterTip')}>
            {t('knowledgeDetails.delimiter')}
          </FormLabel>
          <FormControl>
            <DelimiterInput {...field}></DelimiterInput>
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
