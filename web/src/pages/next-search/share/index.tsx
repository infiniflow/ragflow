import i18n from '@/locales/config';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import {
  ISearchAppDetailProps,
  useFetchSearchDetail,
} from '../../next-searches/hooks';
import { useGetSharedSearchParams, useSearching } from '../hooks';
import '../index.less';
import SearchingView from '../search-view';
export default function SearchingPage() {
  const { tenantId, locale } = useGetSharedSearchParams();
  const {
    data: searchData = {
      search_config: { kb_ids: [] },
    } as unknown as ISearchAppDetailProps,
  } = useFetchSearchDetail(tenantId as string);
  const searchingParam = useSearching({
    data: searchData,
  });
  const { t } = useTranslation();

  // useEffect(() => {
  //   if (locale) {
  //     i18n.changeLanguage(locale);
  //   }
  // }, [locale, i18n]);
  useEffect(() => {
    console.log('locale', locale, i18n.language);
    if (locale && i18n.language !== locale) {
      i18n.changeLanguage(locale);
    }
  }, [locale]);
  return <SearchingView {...searchingParam} searchData={searchData} t={t} />;
}
