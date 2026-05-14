import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import EmbedDialog from '@/components/embed-dialog';
import { useShowEmbedModal } from '@/components/embed-dialog/use-show-embed-dialog';
import { MoreButton } from '@/components/more-button';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { SearchInput } from '@/components/ui/input';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { SharedFrom } from '@/constants/chat';
import { useSetModalState } from '@/hooks/common-hooks';
import {
  useFetchChat,
  useGetChatSearchParams,
  useRemoveSessions,
} from '@/hooks/use-chat-request';
import {
  LucideCopyX,
  LucideListChecks,
  LucidePanelLeftClose,
  LucidePlus,
  LucideSend,
  LucideTrash2,
  LucideUndo2,
} from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';
import { useChatUrlParams } from '../hooks/use-chat-url';
import { useHandleClickConversationCard } from '../hooks/use-click-card';
import { useSelectDerivedConversationList } from '../hooks/use-select-conversation-list';
import { ConversationDropdown } from './conversation-dropdown';

type SessionProps = Pick<
  ReturnType<typeof useHandleClickConversationCard>,
  'handleConversationCardClick'
>;
export function Sessions({ handleConversationCardClick }: SessionProps) {
  const { t } = useTranslation();
  const {
    list: conversationList,
    addTemporaryConversation,
    removeTemporaryConversation,
    handleInputChange,
    searchString,
  } = useSelectDerivedConversationList();
  const { data } = useFetchChat();
  const { visible, switchVisible } = useSetModalState(true);
  const { removeSessions } = useRemoveSessions();
  const { setConversationBoth } = useChatUrlParams();
  const { conversationId } = useGetChatSearchParams();

  // Selection mode state
  const [selectionMode, setSelectionMode] = useState(false);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());

  // Toggle selection mode (click batch delete icon)
  const toggleSelectionMode = useCallback(() => {
    setSelectionMode(true);
    setSelectedIds(new Set());
  }, []);

  // Exit selection mode (click return icon)
  const exitSelectionMode = useCallback(() => {
    setSelectionMode(false);
    setSelectedIds(new Set());
  }, []);

  // Toggle single item selection
  const toggleSelection = useCallback((id: string) => {
    setSelectedIds((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(id)) {
        newSet.delete(id);
      } else {
        newSet.add(id);
      }
      return newSet;
    });
  }, []);

  // Toggle select all
  const toggleSelectAll = useCallback(() => {
    setSelectedIds((prev) => {
      if (prev.size === conversationList.length) {
        return new Set();
      }
      return new Set(conversationList.map((x) => x.id));
    });
  }, [conversationList]);

  // Batch delete
  const handleBatchDelete = useCallback(async () => {
    if (selectedIds.size === 0) {
      return;
    }

    const selectedIdList = Array.from(selectedIds);
    const currentConversationDeleted = conversationId
      ? selectedIdList.includes(conversationId)
      : false;
    const temporaryIdSet = new Set(
      conversationList.filter((item) => item.is_new).map((item) => item.id),
    );
    const persistedIds: string[] = [];

    selectedIdList.forEach((id) => {
      if (temporaryIdSet.has(id)) {
        removeTemporaryConversation(id);
      } else {
        persistedIds.push(id);
      }
    });

    let removeCode = -1;
    if (persistedIds.length > 0) {
      removeCode = await removeSessions(persistedIds);
    }

    if (currentConversationDeleted && conversationId) {
      const currentIsTemporary = temporaryIdSet.has(conversationId);
      const currentPersistedDeleted =
        persistedIds.includes(conversationId) && removeCode === 0;
      if (currentIsTemporary || currentPersistedDeleted) {
        setConversationBoth('', '');
      }
    }
    exitSelectionMode();
  }, [
    selectedIds,
    conversationId,
    conversationList,
    setConversationBoth,
    removeTemporaryConversation,
    removeSessions,
    exitSelectionMode,
  ]);

  const selectedCount = useMemo(() => selectedIds.size, [selectedIds]);

  const { id } = useParams();
  const { showEmbedModal, hideEmbedModal, embedVisible, beta } =
    useShowEmbedModal();

  if (!visible) {
    return (
      <div className="p-5">
        <Button
          variant="transparent"
          size="icon-sm"
          className="border-0"
          onClick={switchVisible}
          data-testid="chat-detail-sessions-open"
        >
          <RAGFlowAvatar
            avatar={data.icon}
            name={data.name}
            className="size-8 cursor-pointer"
          />
        </Button>
      </div>
    );
  }

  return (
    <aside
      className="p-5 w-[296px] flex flex-col"
      role="complementary"
      data-testid="chat-detail-sessions"
    >
      <header className="flex items-center text-base justify-between gap-4">
        <div className="flex gap-3 items-center min-w-0">
          <RAGFlowAvatar
            avatar={data.icon}
            name={data.name}
            className="size-8"
          />

          <span className="flex-1 truncate">{data.name}</span>
        </div>

        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              onClick={showEmbedModal}
              size="icon-xs"
              data-testid="chat-detail-embed-open"
            >
              <LucideSend />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('common.embedIntoSite')}</TooltipContent>
        </Tooltip>

        <EmbedDialog
          visible={embedVisible}
          hideModal={hideEmbedModal}
          token={id!}
          from={SharedFrom.Chat}
          beta={beta}
          isAgent={false}
        />

        <Button
          variant="transparent"
          size="icon-sm"
          className="border-0 ml-auto"
          onClick={switchVisible}
          data-testid="chat-detail-sessions-close"
        >
          <LucidePanelLeftClose />
        </Button>
      </header>

      <div className="flex justify-between items-center mb-4 pt-10">
        <div className="flex items-center gap-3">
          <span className="text-base font-bold">{t('chat.conversations')}</span>
          <data
            className="text-text-secondary text-xs"
            value={conversationList.length}
          >
            {conversationList.length}
          </data>
        </div>

        <div className="flex items-center gap-2">
          {selectionMode ? (
            // Exit selection mode
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={exitSelectionMode}
              data-testid="chat-detail-session-selection-exit"
            >
              <LucideUndo2 size={16} />
            </Button>
          ) : (
            // New conversation
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={addTemporaryConversation}
              data-testid="chat-detail-session-new"
            >
              <LucidePlus className="h-4 w-4" />
            </Button>
          )}

          {selectionMode && selectedCount > 0 ? (
            // Delete selected items
            <ConfirmDeleteDialog
              onOk={handleBatchDelete}
              title={t('chat.batchDeleteSessions')}
              content={{
                title: t('chat.deleteSelectedConfirm', {
                  count: selectedCount,
                }),
              }}
              testId="chat-detail-session-batch-delete-dialog"
              confirmButtonTestId="chat-detail-session-batch-delete-confirm"
              cancelButtonTestId="chat-detail-session-batch-delete-cancel"
            >
              <Button
                variant="delete"
                size="icon-xs"
                data-testid="chat-detail-session-batch-delete"
              >
                <LucideTrash2 />
              </Button>
            </ConfirmDeleteDialog>
          ) : (
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={selectionMode ? toggleSelectAll : toggleSelectionMode}
              data-testid={
                selectionMode
                  ? 'chat-detail-session-select-all'
                  : 'chat-detail-session-selection-enable'
              }
            >
              {selectionMode ? <LucideListChecks /> : <LucideCopyX />}
            </Button>
          )}
        </div>
      </div>

      <div className="pb-4" role="search">
        <SearchInput
          onChange={handleInputChange}
          value={searchString}
          data-testid="chat-detail-session-search"
        ></SearchInput>
      </div>

      <div className="flex-1 overflow-auto">
        {selectionMode ? (
          <ul className="space-y-2" role="listbox" aria-multiselectable>
            {conversationList.map((x) => (
              <li
                key={x.id}
                className="py-2"
                role="option"
                aria-selected={selectedIds.has(x.id)}
                data-session-id={x.id}
              >
                <label className="flex items-center gap-2">
                  <Checkbox
                    checked={selectedIds.has(x.id)}
                    onCheckedChange={() => toggleSelection(x.id)}
                    data-testid="chat-detail-session-checkbox"
                    data-session-id={x.id}
                  />

                  <span className="truncate">{x.name}</span>
                </label>
              </li>
            ))}
          </ul>
        ) : (
          <nav aria-label={t('chat.conversations')}>
            <ul className="space-y-2">
              {conversationList.map((x) => (
                <li
                  key={x.id}
                  className="
                      group pr-3 flex items-center gap-1 rounded-lg
                      aria-selected:bg-bg-card has-[>button:focus-visible]:bg-bg-card
                    "
                  aria-selected={conversationId === x.id}
                >
                  <button
                    type="button"
                    className="focus-visible:outline-none px-3 py-2 text-left flex-1 truncate"
                    onClick={() => handleConversationCardClick(x.id, x.is_new)}
                    data-testid="chat-detail-session-item"
                    data-session-id={x.id}
                  >
                    {x.name}
                  </button>

                  <ConversationDropdown
                    conversation={x}
                    removeTemporaryConversation={removeTemporaryConversation}
                  >
                    <MoreButton
                      data-testid="chat-detail-session-actions"
                      data-session-id={x.id}
                    ></MoreButton>
                  </ConversationDropdown>
                </li>
              ))}
            </ul>
          </nav>
        )}
      </div>
    </aside>
  );
}
