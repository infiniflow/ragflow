import { Dispatch, SetStateAction } from 'react';
import { useTranslation } from 'react-i18next';
import { ISearchAppDetailProps } from '../next-searches/hooks';
import { useSearching } from './hooks';
import './index.less';
import SearchingView from './search-view';
export default function SearchingPage({
  searchText,
  data: searchData,
  setIsSearching,
  setSearchText,
}: {
  searchText: string;
  setIsSearching: Dispatch<SetStateAction<boolean>>;
  setSearchText: Dispatch<SetStateAction<string>>;
  data: ISearchAppDetailProps;
}) {
  const searchingParam = useSearching({
    searchText,
    data: searchData,
    setIsSearching,
    setSearchText,
  });
  const { t } = useTranslation();
  return (
    <SearchingView
      {...searchingParam}
      searchData={searchData}
      setIsSearching={setIsSearching}
      t={t}
    />
  );
}
