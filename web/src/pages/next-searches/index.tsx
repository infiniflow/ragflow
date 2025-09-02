import { IconFont } from '@/components/icon-font';
import ListFilterBar from '@/components/list-filter-bar';
import { RenameDialog } from '@/components/rename-dialog';
import { Button } from '@/components/ui/button';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { useTranslate } from '@/hooks/common-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { pick } from 'lodash';
import { Plus } from 'lucide-react';
import { useState } from 'react';
import { z } from 'zod';
import {
  ISearchAppProps,
  useCreateSearch,
  useFetchSearchList,
  useRenameSearch,
} from './hooks';
import { SearchCard } from './search-card';
const searchFormSchema = z.object({
  name: z.string().min(1, {
    message: 'Name is required',
  }),
});

type SearchFormValues = z.infer<typeof searchFormSchema> & {
  search_id?: string;
};

export default function SearchList() {
  // const { data } = useFetchFlowList();
  const { t } = useTranslate('search');
  const { navigateToSearch } = useNavigatePage();
  const { isLoading, createSearch } = useCreateSearch();
  const [isEdit, setIsEdit] = useState(false);
  const [searchData, setSearchData] = useState<ISearchAppProps | null>(null);
  const {
    data: list,
    searchParams,
    setSearchListParams,
    refetch: refetchList,
  } = useFetchSearchList();
  const {
    openCreateModal,
    showSearchRenameModal,
    hideSearchRenameModal,
    searchRenameLoading,
    onSearchRenameOk,
    initialSearchName,
  } = useRenameSearch();
  const handleSearchChange = (value: string) => {
    console.log(value);
  };
  const onSearchRenameConfirm = (name: string) => {
    onSearchRenameOk(name, () => {
      refetchList();
    });
  };
  const openCreateModalFun = () => {
    setIsEdit(false);
    showSearchRenameModal();
  };
  const handlePageChange = (page: number, pageSize: number) => {
    setIsEdit(false);
    setSearchListParams({ ...searchParams, page, page_size: pageSize });
  };

  return (
    <section className="w-full h-full flex flex-col">
      <div className="px-8 pt-8">
        <ListFilterBar
          icon="search"
          title="Search apps"
          showFilter={false}
          onSearchChange={(e) => handleSearchChange(e.target.value)}
        >
          <Button
            variant={'default'}
            onClick={() => {
              openCreateModalFun();
            }}
          >
            <Plus className="mr-2 h-4 w-4" />
            {t('createSearch')}
          </Button>
        </ListFilterBar>
      </div>
      <div className="flex-1">
        <div className="grid gap-6 sm:grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 max-h-[calc(100dvh-280px)] overflow-auto px-8">
          {list?.data.search_apps.map((x) => {
            return (
              <SearchCard
                key={x.id}
                data={x}
                showSearchRenameModal={() => {
                  showSearchRenameModal(x);
                }}
              ></SearchCard>
            );
          })}
        </div>
      </div>
      {list?.data.total && list?.data.total > 0 && (
        <div className="px-8 mb-4">
          <RAGFlowPagination
            {...pick(searchParams, 'current', 'pageSize')}
            total={list?.data.total}
            onChange={handlePageChange}
          />
        </div>
      )}

      {openCreateModal && (
        <RenameDialog
          hideModal={hideSearchRenameModal}
          onOk={onSearchRenameConfirm}
          initialName={initialSearchName}
          loading={searchRenameLoading}
          title={<IconFont name="search" className="size-6"></IconFont>}
        ></RenameDialog>
      )}
    </section>
  );
}
