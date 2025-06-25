import { FormLabel } from '@/components/ui/form';
import { MultiSelect } from '@/components/ui/multi-select';
import { useTranslation } from 'react-i18next';

const Languages = [
  'English',
  'Chinese',
  'Spanish',
  'French',
  'German',
  'Japanese',
  'Korean',
];

const options = Languages.map((x) => ({ label: x, value: x }));

type CrossLanguageItemProps = {
  name?: string | Array<string>;
  onChange: (arg: string[]) => void;
};

export const CrossLanguageItem = ({
  name = ['prompt_config', 'cross_languages'],
  onChange = () => {},
}: CrossLanguageItemProps) => {
  const { t } = useTranslation();

  return (
    <div>
      <div className="pb-2">
        <FormLabel tooltip={t('chat.crossLanguageTip')}>
          {t('chat.crossLanguage')}
        </FormLabel>
      </div>
      <MultiSelect
        options={options}
        onValueChange={(val) => {
          onChange(val);
        }}
        //   defaultValue={field.value}
        placeholder={t('fileManager.pleaseSelect')}
        maxCount={100}
        //   {...field}
        modalPopover
      />
    </div>
  );
};
