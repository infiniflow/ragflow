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

import Spotlight from '@/components/spotlight';
import message from '@/components/ui/message';
import { useAutoResizeTextarea } from '@/hooks/use-auto-resize-textarea';
import { IUserInfo } from '@/interfaces/database/user-setting';
import { cn } from '@/lib/utils';
import { isEmpty, trim } from 'lodash';
import { Search } from 'lucide-react';
import { Dispatch, SetStateAction, useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import './index.less';
import { RAGFlowLogo } from './ragflow-logo';

export default function SearchHome({
  isSearching,
  setIsSearching,
  searchText,
  setSearchText,
  userInfo,
  canSearch,
  showEmbedLogo,
}: {
  isSearching: boolean;
  setIsSearching: Dispatch<SetStateAction<boolean>>;
  searchText: string;
  setSearchText: Dispatch<SetStateAction<string>>;
  userInfo?: IUserInfo;
  canSearch?: boolean;
  showEmbedLogo?: boolean;
}) {
  // const { data: userInfo } = useFetchUserInfo();
  const { t } = useTranslation();
  const searchInputRef = useRef<HTMLTextAreaElement>(null);

  const isMultiLine = useAutoResizeTextarea(searchInputRef, searchText);

  const handleStartSearch = useCallback(() => {
    if (canSearch === false) {
      message.warning(t('search.chooseDataset'));
      return;
    }
    // Do not enter the searching view with an empty question: it would render
    // the previous search's stale results from the mutation cache.
    if (isEmpty(trim(searchText))) {
      return;
    }
    setIsSearching(!isSearching);
  }, [canSearch, isSearching, searchText, setIsSearching, t]);

  return (
    <section className="relative w-full flex transition-all justify-center items-center mt-[15vh]">
      <div className="relative z-10 px-8 pt-8 flex  text-transparent flex-col justify-center items-center w-full max-w-[780px]">
        <RAGFlowLogo showEmbedIcon={showEmbedLogo}></RAGFlowLogo>
        <div className="rounded-lg  text-primary text-xl sticky flex justify-center w-full transform scale-100 mt-8 p-6 min-h-[240px] border">
          {!isSearching && <Spotlight className="z-0" />}
          <div className="flex flex-col justify-center items-center  w-2/3">
            {!isSearching && (
              <>
                <p className="mb-4 transition-opacity">👋 Hi there</p>
                <p className="mb-10 transition-opacity">
                  {userInfo && (
                    <>
                      {t('search.welcomeBack')}, {userInfo.nickname}
                    </>
                  )}
                </p>
              </>
            )}

            <div className="relative w-full ">
              <textarea
                ref={searchInputRef}
                rows={1}
                placeholder={t('search.searchGreeting')}
                className={cn(
                  'w-full py-4 px-4 pr-14 text-text-primary text-lg bg-bg-base border border-border-button resize-none scrollbar-thin outline-none focus-visible:ring-1 focus-visible:ring-text-primary/50',
                  isMultiLine ? 'rounded-3xl' : 'rounded-full',
                )}
                value={searchText}
                onKeyDown={(e) => {
                  if (
                    e.key === 'Enter' &&
                    !e.shiftKey &&
                    !e.nativeEvent.isComposing
                  ) {
                    e.preventDefault();
                    handleStartSearch();
                  }
                }}
                onChange={(e) => {
                  if (canSearch === false) {
                    message.warning(t('search.chooseDataset'));
                    return;
                  }
                  setSearchText(e.target.value || '');
                }}
              />
              <button
                type="button"
                className={cn(
                  'absolute right-3 flex size-9 items-center justify-center rounded-full bg-text-primary text-bg-base shadow transition-opacity hover:opacity-90',
                  isMultiLine ? 'bottom-3' : 'top-1/2 -translate-y-1/2',
                )}
                onClick={handleStartSearch}
              >
                <Search size={18} />
              </button>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
