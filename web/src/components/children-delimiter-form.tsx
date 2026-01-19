import { cn } from '@/lib/utils';
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
import { Switch } from './ui/switch';

interface IProps {
  value?: string | undefined;
  onChange?: (val: string | undefined) => void;
}

export const DelimiterInput = forwardRef<HTMLInputElement, InputProps & IProps>(
  ({ value, onChange, maxLength, defaultValue, ...props }, ref) => {
    const nextValue = value
      ?.replaceAll('\n', '\\n')
      .replaceAll('\t', '\\t')
      .replaceAll('\r', '\\r');
    const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
      const val = e.target.value;
      const nextValue = val
        .replaceAll('\\n', '\n')
        .replaceAll('\\t', '\t')
        .replaceAll('\\r', '\r');
      onChange?.(nextValue);
    };
    return (
      <Input
        value={nextValue}
        onChange={handleInputChange}
        maxLength={maxLength}
        defaultValue={defaultValue}
        ref={ref}
        className={cn('bg-bg-base', props.className)}
        {...props}
      />
    );
  },
);

export function ChildrenDelimiterForm() {
  const { t } = useTranslation();
  const form = useFormContext();

  const delimiterValue = form.watch('parser_config.children_delimiter');

  return (
    <fieldset className="space-y-2">
      <FormField
        control={form.control}
        name="parser_config.enable_children"
        render={({ field: { value, onChange, ...restProps } }) => (
          <FormItem className="items-center space-y-0 ">
            <div className="flex items-center justify-between gap-1">
              <FormLabel>
                {t('knowledgeDetails.enableChildrenDelimiter')}
              </FormLabel>

              <div className="flex-none">
                <FormControl>
                  <Switch
                    checked={value}
                    onCheckedChange={(checked) => {
                      if (checked && !delimiterValue) {
                        form.setValue('parser_config.children_delimiter', '\n');
                      }

                      onChange(checked);
                    }}
                    {...restProps}
                  />
                </FormControl>
              </div>
            </div>
          </FormItem>
        )}
      />

      {form.getValues('parser_config.enable_children') && (
        <FormField
          control={form.control}
          name="parser_config.children_delimiter"
          render={({ field }) => (
            <FormItem className="items-center space-y-0 ">
              <div className="flex items-center gap-1">
                <FormLabel
                  required
                  tooltip={t('knowledgeDetails.childrenDelimiterTip')}
                  className="text-sm text-text-secondary whitespace-break-spaces w-1/4"
                >
                  {t('knowledgeDetails.childrenDelimiter')}
                </FormLabel>
                <div className="w-3/4">
                  <FormControl>
                    <DelimiterInput {...field} />
                  </FormControl>
                </div>
              </div>
              <div className="flex pt-1">
                <div className="w-1/4"></div>
                <FormMessage />
              </div>
            </FormItem>
          )}
        />
      )}
    </fieldset>
  );
}
