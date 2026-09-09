/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

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
