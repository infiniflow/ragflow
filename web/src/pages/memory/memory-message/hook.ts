import message from '@/components/ui/message';
import { useHandleSearchChange } from '@/hooks/logic-hooks';
import memoryService, { getMemoryDetailById } from '@/services/memory-service';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { t } from 'i18next';
import { useCallback, useState } from 'react';
import { useParams, useSearchParams } from 'umi';
import { MemoryApiAction } from '../constant';
import {
  IMessageContentProps,
  IMessageTableProps,
} from '../memory-message/interface';
import { IMessageInfo } from './interface';

export const useFetchMemoryMessageList = () => {
  const { id } = useParams();
  const [searchParams] = useSearchParams();
  const memoryBaseId = searchParams.get('id') || id;
  const { handleInputChange, searchString, pagination, setPagination } =
    useHandleSearchChange();

  let queryKey: (MemoryApiAction | number)[] = [
    MemoryApiAction.FetchMemoryMessage,
  ];

  const { data, isFetching: loading } = useQuery<IMessageTableProps>({
    queryKey: [...queryKey, searchString, pagination],
    initialData: {} as IMessageTableProps,
    gcTime: 0,
    queryFn: async () => {
      if (memoryBaseId) {
        const { data } = await getMemoryDetailById(memoryBaseId as string, {
          keyword: searchString,
          page: pagination.current,
          page_size: pagination.pageSize,
        });
        return data?.data ?? {};
      } else {
        return {};
      }
    },
  });

  return {
    data,
    loading,
    handleInputChange,
    searchString,
    pagination,
    setPagination,
  };
};

export const useMessageAction = () => {
  const queryClient = useQueryClient();
  const [selectedMessage, setSelectedMessage] = useState<IMessageInfo>(
    {} as IMessageInfo,
  );
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const handleClickDeleteMessage = useCallback((message: IMessageInfo) => {
    console.log('handleClickDeleteMessage', message);
    setSelectedMessage(message);
    setShowDeleteDialog(true);
  }, []);

  const handleDeleteMessage = useCallback(() => {
    // delete message
    memoryService.deleteMemoryMessage(selectedMessage.message_id).then(() => {
      message.success(t('message.deleted'));
      queryClient.invalidateQueries({
        queryKey: [MemoryApiAction.FetchMemoryMessage],
      });
    });
    setShowDeleteDialog(false);
  }, [selectedMessage.message_id, queryClient]);

  const [showMessageContentDialog, setShowMessageContentDialog] =
    useState(false);
  const [selectedMessageContent, setSelectedMessageContent] =
    useState<IMessageContentProps>({} as IMessageContentProps);

  const {
    data: messageContent,
    isPending: fetchMessageContentLoading,
    mutateAsync: fetchMessageContent,
  } = useMutation<IMessageContentProps>({
    mutationKey: [
      MemoryApiAction.FetchMessageContent,
      selectedMessage.message_id,
    ],
    mutationFn: async () => {
      setShowMessageContentDialog(true);
      const res = await memoryService.getMessageContent(
        selectedMessage.message_id,
      );
      if (res.data.code === 0) {
        setSelectedMessageContent(res.data.data);
      } else {
        message.error(res.data.message);
      }
      return res.data.data;
    },
  });

  const handleClickMessageContentDialog = useCallback(
    (message: IMessageInfo) => {
      setSelectedMessage(message);
      fetchMessageContent();
    },
    [fetchMessageContent],
  );

  return {
    selectedMessage,
    setSelectedMessage,
    showDeleteDialog,
    setShowDeleteDialog,
    handleClickDeleteMessage,
    handleDeleteMessage,
    messageContent,
    fetchMessageContentLoading,
    fetchMessageContent,
    selectedMessageContent,
    showMessageContentDialog,
    setShowMessageContentDialog,
    handleClickMessageContentDialog,
  };
};
