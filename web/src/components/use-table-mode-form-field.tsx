import { useTranslate } from '@/hooks/common-hooks';
import { useFormContext } from 'react-hook-form';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from './ui/form';
import { Switch } from './ui/switch';

export function UseTableModeFormField() {
  const form = useFormContext();
  const { t } = useTranslate('knowledgeDetails');

  return (
    <FormField
      control={form.control}
      name="parser_config.use_table_mode"
      render={({ field }) => {
        if (typeof field.value === 'undefined') {
          // default value set
          form.setValue('parser_config.use_table_mode', false);
        }

        return (
          <FormItem defaultChecked={false} className=" items-center space-y-0 ">
            <div className="flex items-center gap-1">
              <FormLabel
                tooltip={t('useTableModeTip')}
                className="text-sm text-text-secondary whitespace-break-spaces w-1/4"
              >
                {t('useTableMode')}
              </FormLabel>
              <div className="w-3/4">
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={(checked) => {
                      field.onChange(checked);
                      // Disable html4excel when use_table_mode is enabled
                      if (checked) {
                        form.setValue('parser_config.html4excel', false);
                      }
                    }}
                  ></Switch>
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
