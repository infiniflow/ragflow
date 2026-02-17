import { CardContainer } from '@/components/card-container';
import { EmptyCardType } from '@/components/empty/constant';
import { EmptyAppCard } from '@/components/empty/empty';
import ListFilterBar from '@/components/list-filter-bar';
import { RenameDialog } from '@/components/rename-dialog';
import { Button } from '@/components/ui/button';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { useTranslate } from '@/hooks/common-hooks';
import { pick } from 'lodash';
import { Plus } from 'lucide-react';
import { useCallback, useEffect } from 'react';
import { useSearchParams } from 'react-router';
import { useFetchSearchList, useRenameSearch } from './hooks';
import { SearchCard } from './search-card';

export default function SearchList() {
  // const { data } = useFetchFlowList();
  const { t } = useTranslate('search');
  // const [isEdit, setIsEdit] = useState(false);
  const {
    data: list,
    pagination,
    searchString,
    handleInputChange,
    setPagination,
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

  // const handleSearchChange = (value: string) => {
  //   console.log(value);
  // };
  const onSearchRenameConfirm = (name: string) => {
    onSearchRenameOk(name, () => {
      refetchList();
    });
  };
  const openCreateModalFun = useCallback(() => {
    // setIsEdit(false);
    showSearchRenameModal();
  }, [showSearchRenameModal]);
  const handlePageChange = useCallback(
    (page: number, pageSize?: number) => {
      setPagination({ page, pageSize });
    },
    [setPagination],
  );

  const [searchUrl, setSearchUrl] = useSearchParams();
  const isCreate = searchUrl.get('isCreate') === 'true';
  useEffect(() => {
    if (isCreate) {
      openCreateModalFun();
      searchUrl.delete('isCreate');
      setSearchUrl(searchUrl);
    }
  }, [isCreate, openCreateModalFun, searchUrl, setSearchUrl]);

  return (
    <section className="w-full h-full flex flex-col">
      {(!list?.data?.search_apps?.length ||
        list?.data?.search_apps?.length <= 0) &&
        !searchString && (
          <div className="flex w-full items-center justify-center h-[calc(100vh-164px)]">
            <EmptyAppCard
              showIcon
              size="large"
              className="w-[480px] p-14"
              type={EmptyCardType.Search}
              isSearch={!!searchString}
              onClick={() => openCreateModalFun()}
            />
          </div>
        )}
      {(!!list?.data?.search_apps?.length || searchString) && (
        <>
          <div className="px-8 pt-8">
            <ListFilterBar
              icon="searches"
              title={t('searchApps')}
              showFilter={false}
              searchString={searchString}
              onSearchChange={handleInputChange}
            >
              <Button
                variant={'default'}
                onClick={() => {
                  openCreateModalFun();
                }}
              >
                <Plus className="h-4 w-4" />
                {t('createSearch')}
              </Button>
            </ListFilterBar>
          </div>
          {(!list?.data?.search_apps?.length ||
            list?.data?.search_apps?.length <= 0) &&
            searchString && (
              <div className="flex w-full items-center justify-center h-[calc(100vh-164px)]">
                <EmptyAppCard
                  showIcon
                  size="large"
                  className="w-[480px] p-14"
                  type={EmptyCardType.Search}
                  isSearch={!!searchString}
                  onClick={() => openCreateModalFun()}
                />
              </div>
            )}
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
                {...pick(pagination, 'current', 'pageSize')}
                // total={pagination.total}
                total={list?.data.total}
                onChange={handlePageChange}
              />
            </div>
          )}
        </>
      )}
      {openCreateModal && (
        <RenameDialog
          hideModal={hideSearchRenameModal}
          onOk={onSearchRenameConfirm}
          initialName={initialSearchName}
          loading={searchRenameLoading}
          title={initialSearchName || t('createSearch')}
        ></RenameDialog>
      )}
    </section>
  );
}
