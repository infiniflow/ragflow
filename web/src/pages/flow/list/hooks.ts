import { useSetModalState } from '@/hooks/common-hooks';
import { useFetchFlowTemplates, useSetFlow } from '@/hooks/flow-hooks';
import { useHandleSearchChange } from '@/hooks/logic-hooks';
import flowService from '@/services/flow-service';
import { useInfiniteQuery } from '@tanstack/react-query';
import { useDebounce } from 'ahooks';
import { useCallback } from 'react';
import { useNavigate } from 'umi';

export const useFetchDataOnMount = () => {
  const { searchString, handleInputChange } = useHandleSearchChange();
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });

  const PageSize = 30;
  const {
    data,
    error,
    fetchNextPage,
    hasNextPage,
    isFetching,
    isFetchingNextPage,
    status,
  } = useInfiniteQuery({
    queryKey: ['infiniteFetchFlowListTeam', debouncedSearchString],
    queryFn: async ({ pageParam }) => {
      const { data } = await flowService.listCanvasTeam({
        page: pageParam,
        page_size: PageSize,
        keywords: debouncedSearchString,
      });
      const list = data?.data ?? [];
      return list;
    },
    initialPageParam: 1,
    getNextPageParam: (lastPage, pages, lastPageParam) => {
      if (lastPageParam * PageSize <= lastPage.total) {
        return lastPageParam + 1;
      }
      return undefined;
    },
  });
  return {
    data,
    loading: isFetching,
    error,
    fetchNextPage,
    hasNextPage,
    isFetching,
    isFetchingNextPage,
    status,
    handleInputChange,
    searchString,
  };
};

export const useSaveFlow = () => {
  const {
    visible: flowSettingVisible,
    hideModal: hideFlowSettingModal,
    showModal: showFileRenameModal,
  } = useSetModalState();
  const { loading, setFlow } = useSetFlow();
  const navigate = useNavigate();
  const { data: list } = useFetchFlowTemplates();

  const onFlowOk = useCallback(
    async (title: string, templateId: string) => {
      const templateItem = list.find((x) => x.id === templateId);

      let dsl = templateItem?.dsl;
      const ret = await setFlow({
        title,
        dsl,
        avatar: templateItem?.avatar,
      });

      if (ret?.code === 0) {
        hideFlowSettingModal();
        navigate(`/flow/${ret.data.id}`);
      }
    },
    [setFlow, hideFlowSettingModal, navigate, list],
  );

  return {
    flowSettingLoading: loading,
    initialFlowName: '',
    onFlowOk,
    flowSettingVisible,
    hideFlowSettingModal,
    templateList: list,
    showFlowSettingModal: showFileRenameModal,
  };
};
