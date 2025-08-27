import ListFilterBar from '@/components/list-filter-bar';
import { RenameDialog } from '@/components/rename-dialog';
import { Button } from '@/components/ui/button';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { useFetchDialogList } from '@/hooks/use-chat-request';
import { pick } from 'lodash';
import { Plus } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
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

  return (
    <section className="flex flex-col w-full flex-1">
      <div className="px-8 pt-8">
        <ListFilterBar
          title="Chat apps"
          onSearchChange={handleInputChange}
          searchString={searchString}
        >
          <Button onClick={handleShowCreateModal}>
            <Plus className="size-2.5" />
            {t('chat.createChat')}
          </Button>
        </ListFilterBar>
      </div>
      <div className="flex-1 overflow-auto">
        <div className="grid gap-6 sm:grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 max-h-[84vh] overflow-auto px-8">
          {data.dialogs.map((x) => {
            return (
              <ChatCard
                key={x.id}
                data={x}
                showChatRenameModal={showChatRenameModal}
              ></ChatCard>
            );
          })}
        </div>
      </div>
      <div className="mt-8 px-8 pb-8">
        <RAGFlowPagination
          {...pick(pagination, 'current', 'pageSize')}
          total={pagination.total}
          onChange={handlePageChange}
        ></RAGFlowPagination>
      </div>
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
