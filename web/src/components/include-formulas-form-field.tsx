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

export function IncludeFormulasFormField() {
  const form = useFormContext();
  const { t } = useTranslate('knowledgeDetails');

  return (
    <FormField
      control={form.control}
      name="parser_config.include_formulas"
      render={({ field }) => {
        if (typeof field.value === 'undefined') {
          // default value set
          form.setValue('parser_config.include_formulas', false);
        }

        return (
          <FormItem defaultChecked={false} className=" items-center space-y-0 ">
            <div className="flex items-center gap-1">
              <FormLabel
                tooltip={t('includeFormulasTip')}
                className="text-sm text-text-secondary whitespace-break-spaces w-1/4"
              >
                {t('includeFormulas')}
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
