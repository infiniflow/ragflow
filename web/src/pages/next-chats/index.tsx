import { CardContainer } from '@/components/card-container';
import { EmptyCardType } from '@/components/empty/constant';
import { EmptyAppCard } from '@/components/empty/empty';
import { NavGroupListFilterBar } from '@/layouts/components/nav-group-list-filter-bar';
import { PrivilegeManagementDialog } from '@/ee/components/privilege-management/privilege-management-dialog';
import { RenameDialog } from '@/components/rename-dialog';
import { Button } from '@/components/ui/button';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { Spin } from '@/components/ui/spin';
import { ListDeletionKey } from '@/constants/list-deletion';
import { useGoToPreviousPageOnEmpty } from '@/hooks/logic-hooks';
import { useFetchChatList } from '@/hooks/use-chat-request';
import { buildOwnersFilter } from '@/utils/list-filter-util';
import { pick } from 'lodash';
import { Plus } from 'lucide-react';
import { useCallback, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router';
import { ChatCard } from './chat-card';
import { useCreateChatDialog } from './hooks/use-create-chat';
import { useRenameChat } from './hooks/use-rename-chat';
import { useShowPrivilegeDialog } from '@/ee/pages/next-chats/use-show-privilege-dialog';

export default function ChatList() {
  const {
    data,
    setPagination,
    pagination,
    handleInputChange,
    searchString,
    setSearchString,
    filterValue,
    setFilterValue,
    handleFilterSubmit,
    loading,
  } = useFetchChatList();
  const { t } = useTranslation();
  const owners = [
    buildOwnersFilter(data?.chats ?? [], undefined, t('common.owner')),
  ];
  const {
    initialChatName,
    chatRenameVisible,
    showChatRenameModal,
    hideChatRenameModal,
    onChatRenameOk,
    chatRenameLoading,
  } = useRenameChat();
  const {
    createChatVisible,
    showCreateChatModal,
    hideCreateChatModal,
    onCreateChatOk,
    createChatLoading,
  } = useCreateChatDialog();

  const handlePageChange = useCallback(
    (page: number, pageSize?: number) => {
      setPagination({ page, pageSize });
    },
    [setPagination],
  );
  useGoToPreviousPageOnEmpty(data?.chats?.length, loading, {
    deletionKey: ListDeletionKey.ChatList,
    searchString,
    setSearchString,
    filterValue,
    setFilterValue,
  });

  const handleShowCreateModal = useCallback(() => {
    showCreateChatModal();
  }, [showCreateChatModal]);

  const [searchParams, setSearchParams] = useSearchParams();
  const isCreate = searchParams.get('isCreate') === 'true';
  useEffect(() => {
    if (isCreate) {
      handleShowCreateModal();
      searchParams.delete('isCreate');
      setSearchParams(searchParams);
    }
  }, [isCreate, handleShowCreateModal, searchParams, setSearchParams]);

  const {
    handleShowPrivilegeModal,
    privilegeRecord,
    hidePrivilegeModal,
    privilegeModalVisible,
  } = useShowPrivilegeDialog();

  const renameDialogProps = useMemo(() => {
    if (chatRenameVisible) {
      return {
        hideModal: hideChatRenameModal,
        onOk: onChatRenameOk,
        initialName: initialChatName,
        loading: chatRenameLoading,
        title: initialChatName,
      };
    }
    if (createChatVisible) {
      return {
        hideModal: hideCreateChatModal,
        onOk: onCreateChatOk,
        initialName: '',
        loading: createChatLoading,
        title: t('chat.createChat'),
      };
    }
    return null;
  }, [
    chatRenameVisible,
    createChatVisible,
    hideChatRenameModal,
    onChatRenameOk,
    initialChatName,
    chatRenameLoading,
    hideCreateChatModal,
    onCreateChatOk,
    createChatLoading,
    t,
  ]);

  return (
    <>
      <article
        className="size-full min-w-0 flex flex-col"
        data-testid="chats-list"
      >
        <header className="mb-5 min-w-0 px-5 pt-5">
          <NavGroupListFilterBar
            onSearchChange={handleInputChange}
            searchString={searchString}
            filters={owners}
            value={filterValue}
            onChange={handleFilterSubmit}
          >
            <Button
              data-testid="create-chat"
              size="icon"
              onClick={handleShowCreateModal}
              aria-label={t('chat.createChat')}
            >
              <Plus />
            </Button>
          </NavGroupListFilterBar>
        </header>

        {loading && !data?.chats?.length ? (
          <div className="flex flex-1 items-center justify-center">
            <Spin size="large" />
          </div>
        ) : data?.chats?.length ? (
          <>
            <CardContainer className="flex-1 overflow-auto px-5">
              {data?.chats.map((x) => (
                <ChatCard
                  key={x.id}
                  data={x}
                  showChatRenameModal={showChatRenameModal}
                  showPrivilegeModal={handleShowPrivilegeModal(x)}
                />
              ))}
            </CardContainer>

            <footer className="mt-4 px-5 pb-5">
              <RAGFlowPagination
                {...pick(pagination, 'current', 'pageSize')}
                total={pagination.total}
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
              type={EmptyCardType.Chat}
              onClick={searchString ? undefined : handleShowCreateModal}
              testId="chats-empty-create"
            />
          </div>
        )}
      </article>

      {renameDialogProps && (
        <RenameDialog {...renameDialogProps}></RenameDialog>
      )}
      {privilegeModalVisible && (
        <PrivilegeManagementDialog
          hideModal={hidePrivilegeModal}
          initialValues={privilegeRecord}
        ></PrivilegeManagementDialog>
      )}
    </>
  );
}
