import { CardContainer } from '@/components/card-container';
import { EmptyCardType } from '@/components/empty/constant';
import { EmptyAppCard } from '@/components/empty/empty';
import ListFilterBar from '@/components/list-filter-bar';
import { RenameDialog } from '@/components/rename-dialog';
import { Button } from '@/components/ui/button';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { useFetchDialogList } from '@/hooks/use-chat-request';
import { pick } from 'lodash';
import { Plus } from 'lucide-react';
import { useCallback, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router';
import { ChatCard } from './chat-card';
import { useRenameChat } from './hooks/use-rename-chat';

export default function ChatList() {
  const { data, setPagination, pagination, handleInputChange, searchString } =
    useFetchDialogList();
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
    <section className="flex flex-col w-full flex-1">
      {data.dialogs?.length <= 0 && !searchString && (
        <div className="flex w-full items-center justify-center h-[calc(100vh-164px)]">
          <EmptyAppCard
            showIcon
            size="large"
            className="w-[480px] p-14"
            isSearch={!!searchString}
            type={EmptyCardType.Chat}
            onClick={() => handleShowCreateModal()}
          />
        </div>
      )}
      {(data.dialogs?.length > 0 || searchString) && (
        <>
          <div className="px-8 pt-8">
            <ListFilterBar
              title={t('chat.chatApps')}
              icon="chats"
              onSearchChange={handleInputChange}
              searchString={searchString}
            >
              <Button onClick={handleShowCreateModal}>
                <Plus className="h-4 w-4" />
                {t('chat.createChat')}
              </Button>
            </ListFilterBar>
          </div>
          {data.dialogs?.length <= 0 && searchString && (
            <div className="flex w-full items-center justify-center h-[calc(100vh-164px)]">
              <EmptyAppCard
                showIcon
                size="large"
                className="w-[480px] p-14"
                isSearch={!!searchString}
                type={EmptyCardType.Chat}
                onClick={() => handleShowCreateModal()}
              />
            </div>
          )}
          <div className="flex-1 overflow-auto">
            <CardContainer className="max-h-[calc(100dvh-280px)] overflow-auto px-8">
              {data.dialogs.map((x) => {
                return (
                  <ChatCard
                    key={x.id}
                    data={x}
                    showChatRenameModal={showChatRenameModal}
                  ></ChatCard>
                );
              })}
            </CardContainer>
          </div>
          <div className="mt-8 px-8 pb-8">
            <RAGFlowPagination
              {...pick(pagination, 'current', 'pageSize')}
              total={pagination.total}
              onChange={handlePageChange}
            ></RAGFlowPagination>
          </div>
        </>
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
    </section>
  );
}
