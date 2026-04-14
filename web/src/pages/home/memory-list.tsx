import { HomeCard } from '@/components/home-card';
import { MoreButton } from '@/components/more-button';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useEffect } from 'react';
import { AddOrEditModal } from '../memories/add-or-edit-modal';
import { useFetchMemoryList, useRenameMemory } from '../memories/hooks';
import { ICreateMemoryProps } from '../memories/interface';
import { MemoryDropdown } from '../memories/memory-dropdown';

export function MemoryList({
  setListLength,
  setLoading,
}: {
  setListLength: (length: number) => void;
  setLoading?: (loading: boolean) => void;
}) {
  const { data, refetch: refetchList, isLoading } = useFetchMemoryList();
  const { navigateToMemory } = useNavigatePage();
  // const {
  //   openCreateModal,
  //   showSearchRenameModal,
  //   hideSearchRenameModal,
  //   searchRenameLoading,
  //   onSearchRenameOk,
  //   initialSearchName,
  // } = useRenameSearch();
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

  useEffect(() => {
    setListLength(data?.data?.memory_list?.length || 0);
    setLoading?.(isLoading || false);
  }, [data, setListLength, isLoading, setLoading]);
  return (
    <>
      {data?.data.memory_list.slice(0, 10).map((x) => (
        <HomeCard
          key={x.id}
          data={{
            name: x?.name,
            avatar: x?.avatar,
            description: x?.description,
            update_time: x?.create_time,
          }}
          onClick={navigateToMemory(x.id)}
          moreDropdown={
            <MemoryDropdown
              memory={x}
              showMemoryRenameModal={showMemoryRenameModal}
            >
              <MoreButton></MoreButton>
            </MemoryDropdown>
          }
        ></HomeCard>
      ))}
      {openCreateModal && (
        <AddOrEditModal
          initialMemory={initialMemory}
          isCreate={false}
          open={openCreateModal}
          loading={searchRenameLoading}
          onClose={hideMemoryModal}
          onSubmit={onMemoryConfirm}
        />
      )}
    </>
  );
}
