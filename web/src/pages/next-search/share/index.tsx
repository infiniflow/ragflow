import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import i18n from '@/locales/config';
import { useEffect } from 'react';
import {
  ISearchAppDetailProps,
  useFetchSearchDetail,
} from '../../next-searches/hooks';
import { useGetSharedSearchParams, useSearching } from '../hooks';
import '../index.less';
import SearchingView from '../search-view';
export default function SearchingPage() {
  const { tenantId, locale, visibleAvatar } = useGetSharedSearchParams();
  const {
    data: searchData = {
      search_config: { kb_ids: [] },
    } as unknown as ISearchAppDetailProps,
  } = useFetchSearchDetail(tenantId as string);
  const searchingParam = useSearching({
    data: searchData,
  });

  useEffect(() => {
    console.log('locale', locale, i18n.language);
    if (locale && i18n.language !== locale) {
      i18n.changeLanguage(locale);
    }
  }, [locale]);
  return (
    <>
      {visibleAvatar && (
        <div className="flex justify-start items-center gap-1 mx-6 mt-6 text-text-primary">
          <RAGFlowAvatar
            avatar={searchData.avatar}
            name={searchData.name}
          ></RAGFlowAvatar>
          <div>{searchData.name}</div>
        </div>
      )}
      <SearchingView {...searchingParam} searchData={searchData} />;
    </>
  );
}
