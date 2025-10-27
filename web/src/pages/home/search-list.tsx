import { HomeCard } from '@/components/home-card';
import { IconFont } from '@/components/icon-font';
import { MoreButton } from '@/components/more-button';
import { RenameDialog } from '@/components/rename-dialog';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchSearchList, useRenameSearch } from '../next-searches/hooks';
import { SearchDropdown } from '../next-searches/search-dropdown';

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
        <HomeCard
          key={x.id}
          data={x}
          onClick={navigateToSearch(x.id)}
          moreDropdown={
            <SearchDropdown
              dataset={x}
              showSearchRenameModal={showSearchRenameModal}
            >
              <MoreButton></MoreButton>
            </SearchDropdown>
          }
        ></HomeCard>
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
