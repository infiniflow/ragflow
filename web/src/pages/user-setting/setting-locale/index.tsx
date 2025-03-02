import { translationTable } from '@/locales/config';
import TranslationTable from './TranslationTable';

function UserSettingLocale() {
  return (
    <TranslationTable
      data={translationTable}
      languages={[
        'zh',
        'English',
        'Vietnamese',
        'Spanish',
        'zh-TRADITIONAL',
        'ja',
        'pt-br',
      ]}
    />
  );
}

export default UserSettingLocale;
