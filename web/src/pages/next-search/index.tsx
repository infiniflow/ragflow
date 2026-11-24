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

import { PageFrame } from '@/components/page-frame';
import { useSearchSidebarCollapse } from '@/hooks/use-search-sidebar-collapse';
import { useFetchUserInfo } from '@/hooks/use-user-setting-request';
import { useCallback, useEffect, useState } from 'react';
import {
  ISearchAppDetailProps,
  useFetchSearchDetail,
} from '../next-searches/hooks';
import { useCheckSettings } from './hooks';
import './index.less';
import { SearchEmbedButton } from './search-embed-button';
import SearchHome from './search-home';
import { SearchSetting } from './search-setting';
import { SearchSidebar } from './search-sidebar';
import SearchingPage from './searching';

export default function SearchPage() {
  const [isSearching, setIsSearching] = useState(false);
  const { data: searchData } = useFetchSearchDetail();

  const [openSetting, setOpenSetting] = useState(false);
  const [searchText, setSearchText] = useState('');
  const { data: userInfo } = useFetchUserInfo();
  const { collapsed } = useSearchSidebarCollapse();
  const { openSetting: settingsRequired } = useCheckSettings(
    searchData as ISearchAppDetailProps,
  );

  useEffect(() => {
    setOpenSetting(settingsRequired);
  }, [settingsRequired]);

  useEffect(() => {
    if (isSearching) {
      setOpenSetting(false);
    }
  }, [isSearching]);

  const handleShowSearch = useCallback(() => {
    setOpenSetting(false);
  }, []);

  const handleShowSettings = useCallback(() => {
    setOpenSetting(true);
  }, []);

  return (
    <section className="flex size-full" data-testid="search-detail">
      {!collapsed && (
        <SearchSidebar
          title={searchData?.name ?? ''}
          settingsActive={openSetting}
          onShowSearch={handleShowSearch}
          onShowSettings={handleShowSettings}
        />
      )}

      <section className="min-h-0 min-w-0 flex-1 pt-2.5">
        <PageFrame>
          <header className="flex shrink-0 justify-end p-5">
            <SearchEmbedButton />
          </header>

          <div
            className="min-h-0 flex-1"
            data-testid="search-detail-content-frame"
          >
            {openSetting && searchData ? (
              <SearchSetting
                data={searchData as ISearchAppDetailProps}
                onClose={handleShowSearch}
              />
            ) : (
              <>
                {!isSearching && (
                  <div className="animate-fade-in-down h-full overflow-x-hidden overflow-y-auto">
                    <SearchHome
                      setIsSearching={setIsSearching}
                      isSearching={isSearching}
                      searchText={searchText}
                      setSearchText={setSearchText}
                      userInfo={userInfo}
                      canSearch={!settingsRequired}
                      showLogo={false}
                    />
                  </div>
                )}
                {isSearching && (
                  <div className="animate-fade-in-up h-full">
                    <SearchingPage
                      setIsSearching={setIsSearching}
                      searchText={searchText}
                      setSearchText={setSearchText}
                      data={searchData as ISearchAppDetailProps}
                      showLogo={false}
                    />
                  </div>
                )}
              </>
            )}
          </div>
        </PageFrame>
      </section>
    </section>
  );
}
