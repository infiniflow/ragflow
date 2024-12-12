import { translationTable } from '@/locales/config';
import TranslationTable from './TranslationTable';

function UserSettingLocale() {
  return (
    <TranslationTable
      data={translationTable}
      languages={[
        'English',
        'Vietnamese',
        'Spanish',
        'zh',
        'zh-TRADITIONAL',
        'ja',
      ]}
    />
  );
}

export default UserSettingLocale;
