import { CardContainer } from '@/components/card-container';
import { EmptyCardType } from '@/components/empty/constant';
import { EmptyAppCard } from '@/components/empty/empty';
import ListFilterBar from '@/components/list-filter-bar';
import { Button } from '@/components/ui/button';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { useTranslate } from '@/hooks/common-hooks';
import { pick } from 'lodash';
import { Plus } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useSearchParams } from 'react-router';
import { AddOrEditModal } from './add-or-edit-modal';
import { defaultMemoryFields } from './constants';
import { useFetchMemoryList, useRenameMemory, useSelectFilters } from './hooks';
import { ICreateMemoryProps, IMemory } from './interface';
import { MemoryCard } from './memory-card';

export default function MemoryList() {
  // const { data } = useFetchFlowList();
  const { t } = useTranslate('memories');
  const [addOrEditType, setAddOrEditType] = useState<'add' | 'edit'>('add');
  // const [isEdit, setIsEdit] = useState(false);
  const {
    data: list,
    pagination,
    searchString,
    handleInputChange,
    setPagination,
    refetch: refetchList,
    filterValue,
    handleFilterSubmit,
  } = useFetchMemoryList();

  const {
    openCreateModal,
    showMemoryRenameModal,
    hideMemoryModal,
    searchRenameLoading,
    onMemoryRenameOk,
    initialMemory,
  } = useRenameMemory();

  const onMemoryConfirm = (data: ICreateMemoryProps) => {
    onMemoryRenameOk(data, () => {
      refetchList();
    });
  };
  const openCreateModalFun = useCallback(() => {
    // setIsEdit(false);
    setAddOrEditType('add');
    showMemoryRenameModal(defaultMemoryFields as unknown as IMemory);
  }, [showMemoryRenameModal]);
  const handlePageChange = useCallback(
    (page: number, pageSize?: number) => {
      setPagination({ page, pageSize });
    },
    [setPagination],
  );

  const [searchUrl, setMemoryUrl] = useSearchParams();
  const { filters } = useSelectFilters();
  const isCreate = searchUrl.get('isCreate') === 'true';
  useEffect(() => {
    if (isCreate) {
      openCreateModalFun();
      searchUrl.delete('isCreate');
      setMemoryUrl(searchUrl);
    }
  }, [isCreate, openCreateModalFun, searchUrl, setMemoryUrl]);

  return (
    <>
      {list?.data?.memory_list?.length || searchString ? (
        <article className="size-full flex flex-col" data-testid="memory-list">
          <header className="px-5 pt-8 mb-4">
            <ListFilterBar
              icon="memory"
              title={t('memory')}
              onSearchChange={handleInputChange}
              searchString={searchString}
              filters={filters}
              onChange={handleFilterSubmit}
              value={filterValue}
            >
              <Button onClick={() => openCreateModalFun()}>
                <Plus className="size-[1em]" />
                {t('createMemory')}
              </Button>
            </ListFilterBar>
          </header>

          {list?.data?.memory_list?.length ? (
            <>
              <CardContainer className="flex-1 overflow-auto px-5">
                {list?.data.memory_list.map((x) => (
                  <MemoryCard
                    key={x.id}
                    data={x}
                    showMemoryRenameModal={() => {
                      setAddOrEditType('edit');
                      showMemoryRenameModal(x);
                    }}
                  />
                ))}
              </CardContainer>

              <footer className="mt-4 px-5 pb-5">
                <RAGFlowPagination
                  {...pick(pagination, 'current', 'pageSize')}
                  total={list?.data.total_count}
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
                type={EmptyCardType.Memory}
                onClick={() => openCreateModalFun()}
              />
            </div>
          )}
        </article>
      ) : (
        <article
          className="size-full flex items-center justify-center"
          data-testid="memory-list"
        >
          <EmptyAppCard
            showIcon
            size="large"
            className="w-[480px] p-14"
            type={EmptyCardType.Memory}
            onClick={() => openCreateModalFun()}
          />
        </article>
      )}
      {/* {openCreateModal && (
        <RenameDialog
          hideModal={hideMemoryRenameModal}
          onOk={onMemoryRenameConfirm}
          initialName={initialMemoryName}
          loading={searchRenameLoading}
          title={<HomeIcon name="memory" width={'24'} />}
        ></RenameDialog>
      )} */}
      {openCreateModal && (
        <AddOrEditModal
          initialMemory={initialMemory}
          isCreate={addOrEditType === 'add'}
          open={openCreateModal}
          loading={searchRenameLoading}
          onClose={hideMemoryModal}
          onSubmit={onMemoryConfirm}
        />
      )}
    </>
  );
}
