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
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { Settings } from 'lucide-react';
import { useState } from 'react';
import {
  ISearchAppDetailProps,
  useFetchSearchDetail,
} from '../next-searches/hooks';
import './index.less';
import SearchHome from './search-home';
import { SearchSetting } from './search-setting';
import SearchingPage from './searching';

export default function SearchPage() {
  const { navigateToSearchList } = useNavigatePage();
  const [isSearching, setIsSearching] = useState(false);
  const { data: SearchData } = useFetchSearchDetail();

  const [openSetting, setOpenSetting] = useState(false);
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
              />
            </div>
          )}
          {isSearching && (
            <div className="animate-fade-in-up">
              <SearchingPage
                setIsSearching={setIsSearching}
                isSearching={isSearching}
              />
            </div>
          )}
        </div>
        {/* {openSetting && (
          <div className=" w-[440px]"> */}
        <SearchSetting
          className="mt-20 mr-2"
          open={openSetting}
          setOpen={setOpenSetting}
          data={SearchData as ISearchAppDetailProps}
        />
        {/* </div>
        )} */}
      </div>

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
    </section>
  );
}
