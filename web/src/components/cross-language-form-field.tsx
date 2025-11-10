import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
} from '@/components/ui/form';
import { MultiSelect } from '@/components/ui/multi-select';
import { cn } from '@/lib/utils';
import { t } from 'i18next';
import { toLower } from 'lodash';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

const Languages = [
  'English',
  'Chinese',
  'Spanish',
  'French',
  'German',
  'Japanese',
  'Korean',
  'Vietnamese',
];

export const crossLanguageOptions = Languages.map((x) => ({
  label: t('language.' + toLower(x)),
  value: x,
}));

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
              placeholder={t('fileManager.pleaseSelect')}
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
