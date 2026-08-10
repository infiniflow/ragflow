import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
} from '@/components/ui/form';
import { MultiSelect } from '@/components/ui/multi-select';
import { cn } from '@/lib/utils';
import { toLower } from 'lodash';
import { useMemo } from 'react';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

export const Languages = [
  'English',
  'Chinese',
  'Spanish',
  'French',
  'German',
  'Japanese',
  'Korean',
  'Vietnamese',
  'Arabic',
  'Turkish',
  'Dutch',
];

export function useCrossLanguageOptions() {
  const { t } = useTranslation();

  return useMemo(
    () =>
      Languages.map((x) => ({
        label: t('language.' + toLower(x)),
        value: x,
      })),
    [t],
  );
}

type CrossLanguageItemProps = {
  name?: string;
  vertical?: boolean;
  label?: string;
};

export const CrossLanguageFormField = ({
  name = 'prompt_config.cross_languages',
  vertical = true,
  label,
}: CrossLanguageItemProps) => {
  const { t } = useTranslation();
  const form = useFormContext();
  const crossLanguageOptions = useCrossLanguageOptions();

  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem
          className={cn('flex', {
            'gap-2': vertical,
            'flex-col': vertical,
            'justify-between': !vertical,
            'items-center': !vertical,
          })}
        >
          <FormLabel tooltip={t('chat.crossLanguageTip')}>
            {label || t('chat.crossLanguage')}
          </FormLabel>
          <FormControl>
            <MultiSelect
              options={crossLanguageOptions}
              placeholder={t('chat.crossLanguagePlaceholder')}
              maxCount={100}
              {...field}
              onValueChange={field.onChange}
              defaultValue={field.value}
              modalPopover
            />
          </FormControl>
        </FormItem>
      )}
    />
  );
};
