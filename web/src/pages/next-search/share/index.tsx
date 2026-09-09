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

import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import i18n, { changeLanguageAsync } from '@/locales/config';
import { useEffect, useState } from 'react';
import {
  ISearchAppDetailProps,
  useFetchSearchDetail,
} from '../../next-searches/hooks';
import { useCheckSettings, useGetSharedSearchParams } from '../hooks';
import '../index.less';
import SearchHome from '../search-home';
import SearchingPage from '../searching';
export default function ShareSearchPage() {
  const { tenantId, locale, visibleAvatar } = useGetSharedSearchParams();
  const {
    data: searchData = {
      search_config: { kb_ids: [] },
    } as unknown as ISearchAppDetailProps,
  } = useFetchSearchDetail(tenantId as string);
  const [isSearching, setIsSearching] = useState(false);
  const [searchText, setSearchText] = useState('');
  const { openSetting: canSearch } = useCheckSettings(
    searchData as ISearchAppDetailProps,
  );

  useEffect(() => {
    if (locale && i18n.language !== locale) {
      changeLanguageAsync(locale, { persist: false });
    }
  }, [locale]);
  return (
    <>
      {visibleAvatar && (
        <div className="flex justify-start items-center gap-2 mx-6 mt-6 text-text-primary">
          <RAGFlowAvatar
            className="size-6"
            avatar={searchData.avatar}
            name={searchData.name}
          ></RAGFlowAvatar>
          <div>{searchData.name}</div>
        </div>
      )}
      {/* <SearchingView {...searchingParam} searchData={searchData} />; */}
      {!isSearching && (
        <div className="animate-fade-in-down">
          <SearchHome
            setIsSearching={setIsSearching}
            isSearching={isSearching}
            searchText={searchText}
            setSearchText={setSearchText}
            canSearch={!canSearch}
            showEmbedLogo={false}
          />
        </div>
      )}
      {isSearching && (
        <div className="animate-fade-in-up">
          <SearchingPage
            setIsSearching={setIsSearching}
            searchText={searchText}
            setSearchText={setSearchText}
            data={searchData as ISearchAppDetailProps}
            showEmbedLogo={false}
          />
        </div>
      )}
    </>
  );
}
