import { CardContainer } from '@/components/card-container';
import { IconFont } from '@/components/icon-font';
import ListFilterBar from '@/components/list-filter-bar';
import { RenameDialog } from '@/components/rename-dialog';
import { Button } from '@/components/ui/button';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { useTranslate } from '@/hooks/common-hooks';
import { Plus } from 'lucide-react';
import { useFetchSearchList, useRenameSearch } from './hooks';
import { SearchCard } from './search-card';

export default function SearchList() {
  // const { data } = useFetchFlowList();
  const { t } = useTranslate('search');
  // const [isEdit, setIsEdit] = useState(false);
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
    // setIsEdit(false);
    showSearchRenameModal();
  };
  const handlePageChange = (page: number, pageSize: number) => {
    // setIsEdit(false);
    setSearchListParams({ ...searchParams, page, page_size: pageSize });
  };

  return (
    <section className="w-full h-full flex flex-col">
      <div className="px-8 pt-8">
        <ListFilterBar
          icon="searches"
          title={t('searchApps')}
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
        <CardContainer className="max-h-[calc(100dvh-280px)] overflow-auto px-8">
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
        </CardContainer>
      </div>
      {list?.data.total && list?.data.total > 0 && (
        <div className="px-8 mb-4">
          <RAGFlowPagination
            current={searchParams.page}
            pageSize={searchParams.page_size}
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
