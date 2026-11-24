import { SelectWithSearch } from '@/components/originui/select-with-search';
import { changeLanguageAsync, supportedLanguages } from '@/locales/config';
import { useTranslation } from 'react-i18next';

const languageOptions = supportedLanguages.map(({ code, displayName }) => ({
  value: code,
  label: displayName,
}));

export function LanguageSwitch() {
  const { i18n } = useTranslation();

  return (
    <SelectWithSearch
      options={languageOptions}
      value={i18n.language}
      onChange={changeLanguageAsync}
      triggerClassName="h-8 w-fit border-0 !bg-transparent px-0 text-sm shadow-none hover:!bg-transparent focus-visible:!bg-transparent"
      testId="auth-language-switch"
      optionTestIdPrefix="auth-language-option-"
    />
  );
}
