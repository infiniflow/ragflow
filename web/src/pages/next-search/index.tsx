import { Button } from '@/components/ui/button';
import { useFetchUserInfo } from '@/hooks/use-user-setting-request';
import { Settings } from 'lucide-react';
import { useEffect, useState } from 'react';
import {
  ISearchAppDetailProps,
  useFetchSearchDetail,
} from '../next-searches/hooks';
import { useCheckSettings } from './hooks';
import './index.less';
import SearchHome from './search-home';
import { SearchSetting } from './search-setting';
import SearchingPage from './searching';

export default function SearchPage() {
  const [isSearching, setIsSearching] = useState(false);
  const { data: SearchData } = useFetchSearchDetail();

  const [openSetting, setOpenSetting] = useState(false);
  const [searchText, setSearchText] = useState('');
  const { data: userInfo } = useFetchUserInfo();
  const { openSetting: checkOpenSetting } = useCheckSettings(
    SearchData as ISearchAppDetailProps,
  );
  useEffect(() => {
    setOpenSetting(checkOpenSetting);
  }, [checkOpenSetting]);

  useEffect(() => {
    if (isSearching) {
      setOpenSetting(false);
    }
  }, [isSearching]);

  return (
    <section
      className="size-full flex-1 relative px-5 pb-5 flex pt-4"
      data-testid="search-detail"
    >
      <div className="flex gap-3 w-full bg-bg-base border-0.5 border-border-button">
        <div className="flex-1 min-w-0">
          {!isSearching && (
            <div className="animate-fade-in-down">
              <SearchHome
                setIsSearching={setIsSearching}
                isSearching={isSearching}
                searchText={searchText}
                setSearchText={setSearchText}
                userInfo={userInfo}
                canSearch={!checkOpenSetting}
              />
            </div>
          )}
          {isSearching && (
            <div className="animate-fade-in-up h-full">
              <SearchingPage
                setIsSearching={setIsSearching}
                searchText={searchText}
                setSearchText={setSearchText}
                data={SearchData as ISearchAppDetailProps}
              />
            </div>
          )}
        </div>
        {openSetting && (
          <SearchSetting
            open={openSetting}
            setOpen={setOpenSetting}
            data={SearchData as ISearchAppDetailProps}
          />
        )}
      </div>

      <Button
        variant="transparent"
        className="bg-bg-card ml-5"
        onClick={() => setOpenSetting(!openSetting)}
      >
        <Settings className="text-text-secondary" />
      </Button>
    </section>
  );
}
