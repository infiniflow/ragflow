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
    <>
      {list?.data?.search_apps?.length || searchString ? (
        <article className="size-full flex flex-col" data-testid="search-list">
          <header className="px-5 pt-8 mb-4">
            <ListFilterBar
              icon="searches"
              title={t('searchApps')}
              showFilter={false}
              searchString={searchString}
              onSearchChange={handleInputChange}
            >
              <Button
                data-testid="create-search"
                onClick={() => openCreateModalFun()}
              >
                <Plus className="size-[1em]" />
                {t('createSearch')}
              </Button>
            </ListFilterBar>
          </header>

          {list?.data?.search_apps?.length ? (
            <>
              <CardContainer className="flex-1 overflow-auto px-5">
                {list?.data.search_apps.map((x) => {
                  return (
                    <SearchCard
                      key={x.id}
                      data={x}
                      showSearchRenameModal={() => {
                        showSearchRenameModal(x);
                      }}
                    />
                  );
                })}
              </CardContainer>

              <footer className="mt-4 px-5 pb-5">
                <RAGFlowPagination
                  {...pick(pagination, 'current', 'pageSize')}
                  total={list?.data.total}
                  onChange={handlePageChange}
                />
              </footer>
            </>
          ) : (
            <div className="flex-1 flex items-center justify-center">
              <EmptyAppCard
                showIcon
                size="large"
                className="w-[480px] p-14"
                isSearch
                type={EmptyCardType.Search}
                testId="search-empty-create"
              />
            </div>
          )}
        </article>
      ) : (
        <article
          className="size-full flex items-center justify-center"
          data-testid="search-list"
        >
          <EmptyAppCard
            showIcon
            size="large"
            className="w-[480px] p-14"
            type={EmptyCardType.Search}
            onClick={() => openCreateModalFun()}
            testId="search-empty-create"
          />
        </article>
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
    </>
  );
}
