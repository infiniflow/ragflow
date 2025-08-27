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

export function ExcelToHtmlFormField() {
  const form = useFormContext();
  const { t } = useTranslate('knowledgeDetails');

  return (
    <FormField
      control={form.control}
      name="parser_config.html4excel"
      render={({ field }) => {
        if (typeof field.value === 'undefined') {
          // default value set
          form.setValue('parser_config.html4excel', false);
        }

        return (
          <FormItem defaultChecked={false} className=" items-center space-y-0 ">
            <div className="flex items-center gap-1">
              <FormLabel
                tooltip={t('html4excelTip')}
                className="text-sm text-muted-foreground whitespace-break-spaces w-1/4"
              >
                {t('html4excel')}
              </FormLabel>
              <div className="w-3/4">
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
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
