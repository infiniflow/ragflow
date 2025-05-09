import { Select as AntSelect, Form } from 'antd';
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

export const LanguageItem = () => {
  const { t } = useTranslation();

  return (
    <Form.Item
      label={t('chat.crossLanguage')}
      name={['prompt_config', 'cross_languages']}
      tooltip={t('chat.crossLanguageTip')}
    >
      <AntSelect
        options={options}
        allowClear
        placeholder={t('common.languagePlaceholder')}
        mode="multiple"
      />
    </Form.Item>
  );
};
