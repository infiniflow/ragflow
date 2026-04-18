import { Dispatch, SetStateAction } from 'react';
import { ISearchAppDetailProps } from '../next-searches/hooks';
import { useSearching } from './hooks';
import './index.less';
import SearchingView from './search-view';
export default function SearchingPage({
  searchText,
  data: searchData,
  setIsSearching,
  setSearchText,
  showEmbedLogo,
}: {
  searchText: string;
  setIsSearching: Dispatch<SetStateAction<boolean>>;
  setSearchText: Dispatch<SetStateAction<string>>;
  data: ISearchAppDetailProps;
  showEmbedLogo?: boolean;
}) {
  const searchingParam = useSearching({
    searchText,
    data: searchData,
    setIsSearching,
    setSearchText,
  });
  return (
    <SearchingView
      {...searchingParam}
      searchData={searchData}
      setIsSearching={setIsSearching}
      showEmbedLogo={showEmbedLogo}
    />
  );
}
