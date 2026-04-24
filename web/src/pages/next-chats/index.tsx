import { CardContainer } from '@/components/card-container';
import { EmptyCardType } from '@/components/empty/constant';
import { EmptyAppCard } from '@/components/empty/empty';
import ListFilterBar from '@/components/list-filter-bar';
import { RenameDialog } from '@/components/rename-dialog';
import { Button } from '@/components/ui/button';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { useFetchChatList } from '@/hooks/use-chat-request';
import { pick } from 'lodash';
import { Plus } from 'lucide-react';
import { useCallback, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router';
import { ChatCard } from './chat-card';
import { useRenameChat } from './hooks/use-rename-chat';

export default function ChatList() {
  const { data, setPagination, pagination, handleInputChange, searchString } =
    useFetchChatList();
  const { t } = useTranslation();
  const {
    initialChatName,
    chatRenameVisible,
    showChatRenameModal,
    hideChatRenameModal,
    onChatRenameOk,
    chatRenameLoading,
  } = useRenameChat();

  const handlePageChange = useCallback(
    (page: number, pageSize?: number) => {
      setPagination({ page, pageSize });
    },
    [setPagination],
  );

  const handleShowCreateModal = useCallback(() => {
    showChatRenameModal();
  }, [showChatRenameModal]);

  const [searchParams, setSearchParams] = useSearchParams();
  const isCreate = searchParams.get('isCreate') === 'true';
  useEffect(() => {
    if (isCreate) {
      handleShowCreateModal();
      searchParams.delete('isCreate');
      setSearchParams(searchParams);
    }
  }, [isCreate, handleShowCreateModal, searchParams, setSearchParams]);

  return (
    <>
      {data.chats?.length || searchString ? (
        <article className="size-full flex flex-col" data-testid="chats-list">
          <header className="px-5 pt-8 mb-4">
            <ListFilterBar
              title={t('chat.chatApps')}
              icon="chats"
              onSearchChange={handleInputChange}
              searchString={searchString}
            >
              <Button data-testid="create-chat" onClick={handleShowCreateModal}>
                <Plus className="size-[1em]" />
                {t('chat.createChat')}
              </Button>
            </ListFilterBar>
          </header>

          {data.chats?.length ? (
            <>
              <CardContainer className="flex-1 overflow-auto px-5">
                {data.chats.map((x) => (
                  <ChatCard
                    key={x.id}
                    data={x}
                    showChatRenameModal={showChatRenameModal}
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
                isSearch
                type={EmptyCardType.Chat}
                testId="chats-empty-create"
              />
            </div>
          )}
        </article>
      ) : (
        <article
          className="size-full flex items-center justify-center"
          data-testid="chats-list"
        >
          <EmptyAppCard
            showIcon
            size="large"
            className="w-[480px] p-14"
            type={EmptyCardType.Chat}
            onClick={() => handleShowCreateModal()}
            testId="chats-empty-create"
          />
        </article>
      )}

      {chatRenameVisible && (
        <RenameDialog
          hideModal={hideChatRenameModal}
          onOk={onChatRenameOk}
          initialName={initialChatName}
          loading={chatRenameLoading}
          title={initialChatName || t('chat.createChat')}
        ></RenameDialog>
      )}
    </>
  );
}
