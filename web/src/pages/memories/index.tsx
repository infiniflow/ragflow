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
    <section className="w-full h-full flex flex-col">
      {(!list?.data?.memory_list?.length ||
        list?.data?.memory_list?.length <= 0) &&
        !searchString && (
          <div className="flex w-full items-center justify-center h-[calc(100vh-164px)]">
            <EmptyAppCard
              showIcon
              size="large"
              className="w-[480px] p-14"
              isSearch={!!searchString}
              type={EmptyCardType.Memory}
              onClick={() => openCreateModalFun()}
            />
          </div>
        )}
      {(!!list?.data?.memory_list?.length || searchString) && (
        <>
          <div className="px-8 pt-8">
            <ListFilterBar
              icon="memory"
              title={t('memory')}
              onSearchChange={handleInputChange}
              searchString={searchString}
              filters={filters}
              onChange={handleFilterSubmit}
              value={filterValue}
            >
              <Button
                variant={'default'}
                onClick={() => {
                  openCreateModalFun();
                }}
              >
                <Plus className=" h-4 w-4" />
                {t('createMemory')}
              </Button>
            </ListFilterBar>
          </div>
          {(!list?.data?.memory_list?.length ||
            list?.data?.memory_list?.length <= 0) &&
            searchString && (
              <div className="flex w-full items-center justify-center h-[calc(100vh-164px)]">
                <EmptyAppCard
                  showIcon
                  size="large"
                  className="w-[480px] p-14"
                  isSearch={!!searchString}
                  type={EmptyCardType.Memory}
                  onClick={() => openCreateModalFun()}
                />
              </div>
            )}
          <div className="flex-1">
            <CardContainer className="max-h-[calc(100dvh-280px)] overflow-auto px-8">
              {list?.data.memory_list.map((x) => {
                return (
                  <MemoryCard
                    key={x.id}
                    data={x}
                    showMemoryRenameModal={() => {
                      setAddOrEditType('edit');
                      showMemoryRenameModal(x);
                    }}
                  ></MemoryCard>
                );
              })}
            </CardContainer>
          </div>
          {list?.data.total_count && list?.data.total_count > 0 && (
            <div className="px-8 mb-4">
              <RAGFlowPagination
                {...pick(pagination, 'current', 'pageSize')}
                // total={pagination.total}
                total={list?.data.total_count}
                onChange={handlePageChange}
              />
            </div>
          )}
        </>
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
    </section>
  );
}
