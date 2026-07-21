import Spotlight from '@/components/spotlight';
import message from '@/components/ui/message';
import { IUserInfo } from '@/interfaces/database/user-setting';
import { Search } from 'lucide-react';
import { Dispatch, SetStateAction, useEffect, useRef } from 'react';
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

  // Grow the search box with its content so long queries stay readable.
  useEffect(() => {
    const el = searchInputRef.current;
    if (!el) return;
    el.style.height = 'auto';
    el.style.height = `${Math.min(el.scrollHeight, 160)}px`;
  }, [searchText]);
  return (
    <section className="relative w-full flex transition-all justify-center items-center mt-[15vh]">
      <div className="relative z-10 px-8 pt-8 flex  text-transparent flex-col justify-center items-center w-[780px]">
        <RAGFlowLogo showEmbedIcon={showEmbedLogo}></RAGFlowLogo>
        <div className="rounded-lg  text-primary text-xl sticky flex justify-center w-full transform scale-100 mt-8 p-6 h-[240px] border">
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
                className="w-full rounded-3xl py-4 px-4 pr-14 text-text-primary text-lg bg-bg-base delay-700 border border-border-button resize-none overflow-y-auto scrollbar-thin outline-none focus-visible:ring-1 focus-visible:ring-text-primary/50"
                value={searchText}
                onKeyDown={(e) => {
                  if (
                    e.key === 'Enter' &&
                    !e.shiftKey &&
                    !e.nativeEvent.isComposing
                  ) {
                    e.preventDefault();
                    if (canSearch === false) {
                      message.warning(t('search.chooseDataset'));
                      return;
                    }
                    setIsSearching(!isSearching);
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
                className="absolute right-2 top-1/2 -translate-y-1/2 transform rounded-full bg-text-primary p-2 text-bg-base shadow w-12"
                onClick={() => {
                  if (canSearch === false) {
                    message.warning(t('search.chooseDataset'));
                    return;
                  }
                  setIsSearching(!isSearching);
                }}
              >
                <Search size={22} className="m-auto" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
