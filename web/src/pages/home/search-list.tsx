import { IconFont } from '@/components/icon-font';
import { MoreButton } from '@/components/more-button';
import { RenameDialog } from '@/components/rename-dialog';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchSearchList, useRenameSearch } from '../next-searches/hooks';
import { SearchDropdown } from '../next-searches/search-dropdown';
import { ApplicationCard } from './application-card';

export function SearchList() {
  const { data, refetch: refetchList } = useFetchSearchList();
  const { navigateToSearch } = useNavigatePage();
  const {
    openCreateModal,
    showSearchRenameModal,
    hideSearchRenameModal,
    searchRenameLoading,
    onSearchRenameOk,
    initialSearchName,
  } = useRenameSearch();
  const onSearchRenameConfirm = (name: string) => {
    onSearchRenameOk(name, () => {
      refetchList();
    });
  };
  return (
    <>
      {data?.data.search_apps.slice(0, 10).map((x) => (
        <ApplicationCard
          key={x.id}
          app={{
            avatar: x.avatar,
            title: x.name,
            update_time: x.update_time,
          }}
          onClick={navigateToSearch(x.id)}
          moreDropdown={
            <SearchDropdown
              dataset={x}
              showSearchRenameModal={showSearchRenameModal}
            >
              <MoreButton></MoreButton>
            </SearchDropdown>
          }
        ></ApplicationCard>
      ))}
      {openCreateModal && (
        <RenameDialog
          hideModal={hideSearchRenameModal}
          onOk={onSearchRenameConfirm}
          initialName={initialSearchName}
          loading={searchRenameLoading}
          title={<IconFont name="search" className="size-6"></IconFont>}
        ></RenameDialog>
      )}
    </>
  );
}
