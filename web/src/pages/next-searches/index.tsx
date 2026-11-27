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

import { CardContainer } from '@/components/card-container';
import { EmptyCardType } from '@/components/empty/constant';
import { EmptyAppCard } from '@/components/empty/empty';
import { NavGroupListFilterBar } from '@/layouts/components/nav-group-list-filter-bar';
import { PrivilegeManagementDialog } from '@/ee/components/privilege-management/privilege-management-dialog';
import { RenameDialog } from '@/components/rename-dialog';
import { Button } from '@/components/ui/button';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { ListDeletionKey } from '@/constants/list-deletion';
import { useTranslate } from '@/hooks/common-hooks';
import { useGoToPreviousPageOnEmpty } from '@/hooks/logic-hooks';
import { buildOwnersFilter } from '@/utils/list-filter-util';
import { pick } from 'lodash';
import { Plus } from 'lucide-react';
import { useCallback, useEffect } from 'react';
import { useSearchParams } from 'react-router';
import { useFetchSearchList, useRenameSearch } from './hooks';
import { SearchCard } from './search-card';
import { useShowPrivilegeDialog } from '@/ee/pages/next-searches/use-show-privilege-dialog';

export default function SearchList() {
  // const { data } = useFetchFlowList();
  const { t } = useTranslate('search');
  const { t: tc } = useTranslate('common');
  // const [isEdit, setIsEdit] = useState(false);
  const {
    data: list,
    isLoading,
    pagination,
    searchString,
    setSearchString,
    handleInputChange,
    setPagination,
    refetch: refetchList,
    filterValue,
    setFilterValue,
    handleFilterSubmit,
  } = useFetchSearchList();
  const owners = [
    buildOwnersFilter(list?.data?.search_apps ?? [], undefined, tc('owner')),
  ];
  const {
    openCreateModal,
    showSearchRenameModal,
    hideSearchRenameModal,
    searchRenameLoading,
    onSearchRenameOk,
    initialSearchName,
  } = useRenameSearch();
  const {
    privilegeModalVisible,
    hidePrivilegeModal,
    handShowPrivilegeModal,
    recordWithSourceType,
  } = useShowPrivilegeDialog();

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
  useGoToPreviousPageOnEmpty(list?.data?.search_apps?.length, isLoading, {
    deletionKey: ListDeletionKey.SearchList,
    searchString,
    setSearchString,
    filterValue,
    setFilterValue,
  });

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
      <article
        className="size-full min-w-0 flex flex-col"
        data-testid="search-list"
      >
        <header className="mb-5 min-w-0 px-5 pt-5">
          <NavGroupListFilterBar
            searchString={searchString}
            onSearchChange={handleInputChange}
            value={filterValue}
            onChange={handleFilterSubmit}
            filters={owners}
          >
            <Button
              data-testid="create-search"
              size="icon"
              onClick={openCreateModalFun}
              aria-label={t('createSearch')}
            >
              <Plus />
            </Button>
          </NavGroupListFilterBar>
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
                    showPrivilegeModal={handShowPrivilegeModal(x)}
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
              isSearch={Boolean(searchString)}
              type={EmptyCardType.Search}
              onClick={searchString ? undefined : openCreateModalFun}
              testId="search-empty-create"
            />
          </div>
        )}
      </article>
      {openCreateModal && (
        <RenameDialog
          hideModal={hideSearchRenameModal}
          onOk={onSearchRenameConfirm}
          initialName={initialSearchName}
          loading={searchRenameLoading}
          title={initialSearchName || t('createSearch')}
        ></RenameDialog>
      )}
      {privilegeModalVisible && (
        <PrivilegeManagementDialog
          hideModal={hidePrivilegeModal}
          initialValues={recordWithSourceType}
        />
      )}
    </>
  );
}
