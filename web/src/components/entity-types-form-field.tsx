import { useTranslate } from '@/hooks/common-hooks';
import { useFormContext } from 'react-hook-form';
import EditTag from './edit-tag';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from './ui/form';

type EntityTypesFormFieldProps = {
  name?: string;
};

export function EntityTypesFormField({
  name = 'parser_config.entity_types',
}: EntityTypesFormFieldProps) {
  const { t } = useTranslate('knowledgeConfiguration');
  const form = useFormContext();

  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t('entityTypes')}</FormLabel>
          <FormControl>
            <EditTag {...field}></EditTag>
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
