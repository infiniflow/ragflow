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
      render={({ field }) => {
        if (typeof field.value === 'undefined') {
          // default value set
          form.setValue('parser_config.delimiter', '\n');
        }
        return (
          <FormItem className=" items-center space-y-0 ">
            <div className="flex items-center gap-1">
              <FormLabel
                tooltip={t('knowledgeDetails.delimiterTip')}
                className="text-sm text-muted-foreground whitespace-break-spaces w-1/4"
              >
                {t('knowledgeDetails.delimiter')}
              </FormLabel>
              <div className="w-3/4">
                <FormControl>
                  <DelimiterInput {...field}></DelimiterInput>
                </FormControl>
              </div>
            </div>
            <div className="flex pt-1">
              <div className="w-1/4"></div>
              <FormMessage />
            </div>
          </FormItem>
        );
      }}
    />
  );
}
