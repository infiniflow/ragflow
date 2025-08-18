import { PageHeader } from '@/components/page-header';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import { Button } from '@/components/ui/button';
import { SharedFrom } from '@/constants/chat';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchTenantInfo } from '@/hooks/user-setting-hooks';
import { Send, Settings } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useFetchTokenListBeforeOtherStep } from '../agent/hooks/use-show-dialog';
import {
  ISearchAppDetailProps,
  useFetchSearchDetail,
} from '../next-searches/hooks';
import EmbedAppModal from './embed-app-modal';
import './index.less';
import SearchHome from './search-home';
import { SearchSetting } from './search-setting';
import SearchingPage from './searching';

export default function SearchPage() {
  const { navigateToSearchList } = useNavigatePage();
  const [isSearching, setIsSearching] = useState(false);
  const { data: SearchData } = useFetchSearchDetail();
  const { beta, handleOperate } = useFetchTokenListBeforeOtherStep();
  const [openSetting, setOpenSetting] = useState(false);
  const [openEmbed, setOpenEmbed] = useState(false);
  const [searchText, setSearchText] = useState('');
  const { data: tenantInfo } = useFetchTenantInfo();
  const tenantId = tenantInfo.tenant_id;
  useEffect(() => {
    handleOperate();
  }, [handleOperate]);
  useEffect(() => {
    if (isSearching) {
      setOpenSetting(false);
    }
  }, [isSearching]);

  return (
    <section>
      <PageHeader>
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink onClick={navigateToSearchList}>
                Search
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>{SearchData?.name}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
      </PageHeader>
      <div className="flex gap-3 w-full">
        <div className="flex-1">
          {!isSearching && (
            <div className="animate-fade-in-down">
              <SearchHome
                setIsSearching={setIsSearching}
                isSearching={isSearching}
                searchText={searchText}
                setSearchText={setSearchText}
              />
            </div>
          )}
          {isSearching && (
            <div className="animate-fade-in-up">
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
            className="mt-20 mr-2"
            open={openSetting}
            setOpen={setOpenSetting}
            data={SearchData as ISearchAppDetailProps}
          />
        )}
        {
          <EmbedAppModal
            open={openEmbed}
            setOpen={setOpenEmbed}
            url="/next-search/share"
            token={SearchData?.id as string}
            from={SharedFrom.Search}
            beta={beta}
            tenantId={tenantId}
          />
        }
        {
          // <EmbedDialog
          //   visible={openEmbed}
          //   hideModal={setOpenEmbed}
          //   token={SearchData?.id as string}
          //   from={SharedFrom.Search}
          //   beta={beta}
          //   isAgent={false}
          // ></EmbedDialog>
        }
      </div>
      <div className="absolute right-5 top-12 ">
        <Button
          className="bg-text-primary  text-bg-base border-b-[#00BEB4] border-b-2"
          onClick={() => setOpenEmbed(!openEmbed)}
        >
          <Send />
          <div>Embed App</div>
        </Button>
      </div>
      {!isSearching && (
        <div className="absolute left-5 bottom-12 ">
          <Button
            variant="transparent"
            className="bg-bg-card"
            onClick={() => setOpenSetting(!openSetting)}
          >
            <Settings className="text-text-secondary" />
            <div className="text-text-secondary">Search Settings</div>
          </Button>
        </div>
      )}
    </section>
  );
}
