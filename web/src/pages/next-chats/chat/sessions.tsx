import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { MoreButton } from '@/components/more-button';
import { SecondarySidebarHeader } from '@/components/sidebar-header';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { SearchInput } from '@/components/ui/input';
import {
  useFetchChat,
  useGetChatSearchParams,
  useRemoveSessions,
} from '@/hooks/use-chat-request';
import { useChatSidebarCollapse } from '@/hooks/use-chat-sidebar-collapse';
import { useSidebarCollapse } from '@/layouts/components/app-sidebar/use-sidebar-collapse';
import { cn } from '@/lib/utils';
import {
  LucideActivity,
  LucideCopyX,
  LucideListChecks,
  LucideLoader2,
  LucidePlus,
  LucideSettings,
  LucideTrash2,
  LucideUndo2,
} from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useChatStreamStore } from '../chat-stream/store';
import { useChatUrlParams } from '../hooks/use-chat-url';
import { useHandleClickConversationCard } from '../hooks/use-click-card';
import { useSelectDerivedConversationList } from '../hooks/use-select-conversation-list';
import { ConversationDropdown } from './conversation-dropdown';

type SessionProps = Pick<
  ReturnType<typeof useHandleClickConversationCard>,
  'handleConversationCardClick'
> & {
  settingsActive: boolean;
  multipleModelsActive: boolean;
  onOpenSettings: () => void;
  onOpenMultipleModels: () => void;
  onShowSingleChat: () => void;
};

export function Sessions({
  handleConversationCardClick,
  settingsActive,
  multipleModelsActive,
  onOpenSettings,
  onOpenMultipleModels,
  onShowSingleChat,
}: SessionProps) {
  const { t } = useTranslation();
  const {
    list: conversationList,
    total,
    addTemporaryConversation,
    removeTemporaryConversation,
    handleInputChange,
    searchString,
    loadingMore,
    handleScroll,
  } = useSelectDerivedConversationList();
  const { data } = useFetchChat();
  const { collapsed, setCollapsed } = useChatSidebarCollapse();
  const { setCollapsed: setPrimarySidebarCollapsed } = useSidebarCollapse();
  const { removeSessions } = useRemoveSessions();
  const { setConversationBoth } = useChatUrlParams();
  const { conversationId } = useGetChatSearchParams();
  const removeStreamSessions = useChatStreamStore(
    (state) => state.removeSessions,
  );

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

  // Selected items that are still visible under the current search filter.
  // Batch deletion and the selected count must act on this set — selectedIds
  // alone would include items filtered out of view by the search.
  const visibleSelectedIds = useMemo(
    () =>
      conversationList.filter((x) => selectedIds.has(x.id)).map((x) => x.id),
    [conversationList, selectedIds],
  );

  // Toggle select all
  const toggleSelectAll = useCallback(() => {
    setSelectedIds((prev) => {
      const allVisibleSelected =
        conversationList.length > 0 &&
        conversationList.every((x) => prev.has(x.id));
      if (allVisibleSelected) {
        return new Set();
      }
      return new Set(conversationList.map((x) => x.id));
    });
  }, [conversationList]);

  // Batch delete
  const handleBatchDelete = useCallback(async () => {
    if (visibleSelectedIds.length === 0) {
      return;
    }

    const currentConversationDeleted = conversationId
      ? visibleSelectedIds.includes(conversationId)
      : false;
    const temporaryIdSet = new Set(
      conversationList.filter((item) => item.is_new).map((item) => item.id),
    );
    const persistedIds: string[] = [];
    const removedTemporaryIds: string[] = [];

    visibleSelectedIds.forEach((id) => {
      if (temporaryIdSet.has(id)) {
        removeTemporaryConversation(id);
        removedTemporaryIds.push(id);
      } else {
        persistedIds.push(id);
      }
    });

    let removeCode = -1;
    if (persistedIds.length > 0) {
      removeCode = await removeSessions(persistedIds);
    }

    // Purge the streaming store for every id that actually went away, aborting
    // any in-flight stream so it can't keep writing into a dead session.
    removeStreamSessions([
      ...removedTemporaryIds,
      ...(removeCode === 0 ? persistedIds : []),
    ]);

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
    visibleSelectedIds,
    conversationId,
    conversationList,
    setConversationBoth,
    removeTemporaryConversation,
    removeSessions,
    removeStreamSessions,
    exitSelectionMode,
  ]);

  const selectedCount = visibleSelectedIds.length;

  const handleNewConversation = useCallback(() => {
    addTemporaryConversation();
    onShowSingleChat();
  }, [addTemporaryConversation, onShowSingleChat]);

  const handleConversationClick = useCallback(
    (selectedConversationId: string, isNew: boolean) => {
      handleConversationCardClick(selectedConversationId, isNew);
      onShowSingleChat();
    },
    [handleConversationCardClick, onShowSingleChat],
  );

  const collapseSidebars = useCallback(() => {
    setCollapsed(true);
    setPrimarySidebarCollapsed(true);
  }, [setCollapsed, setPrimarySidebarCollapsed]);

  if (collapsed) {
    return null;
  }

  return (
    <aside
      className="w-[296px] flex flex-col"
      role="complementary"
      data-testid="chat-detail-sessions"
    >
      <SecondarySidebarHeader title={data.name} onCollapse={collapseSidebars} />

      <div className="flex min-h-0 flex-1 flex-col px-5 pb-5">
        <nav className="flex flex-col gap-2 pt-5" aria-label={data.name}>
          <Button
            variant="outline"
            size="lg"
            block
            onClick={handleNewConversation}
            data-testid="chat-detail-session-new"
          >
            <LucidePlus />
            {t('chat.newConversation')}
          </Button>

          <Button
            variant={settingsActive ? 'secondary' : 'ghost'}
            size="lg"
            block
            className="justify-start"
            onClick={onOpenSettings}
            aria-pressed={settingsActive}
            data-testid="chat-settings"
          >
            <LucideSettings />
            {t('chat.settings')}
          </Button>

          <Button
            variant={multipleModelsActive ? 'secondary' : 'ghost'}
            size="lg"
            block
            className="justify-start"
            onClick={onOpenMultipleModels}
            aria-pressed={multipleModelsActive}
            data-testid="chat-detail-multimodel-toggle"
          >
            <LucideActivity />
            {t('chat.multipleModels')}
          </Button>
        </nav>

        <div className="flex justify-between items-center mb-4 pt-8">
          <div className="flex items-center gap-3">
            <span className="text-base">{t('chat.conversations')}</span>
            <data className="text-text-secondary text-xs" value={total}>
              {total}
            </data>
          </div>

          <div className="flex items-center gap-2">
            {selectionMode && (
              <Button
                variant="ghost"
                size="icon-xs"
                onClick={exitSelectionMode}
                data-testid="chat-detail-session-selection-exit"
              >
                <LucideUndo2 size={16} />
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

        <div className="flex-1 overflow-auto" onScroll={handleScroll}>
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
                      onClick={() => handleConversationClick(x.id, x.is_new)}
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
                        className={cn(
                          conversationId === x.id ? 'block' : 'hidden',
                        )}
                        data-testid="chat-detail-session-actions"
                        data-session-id={x.id}
                      ></MoreButton>
                    </ConversationDropdown>
                  </li>
                ))}
              </ul>
            </nav>
          )}
          {loadingMore && (
            <LucideLoader2
              className="mx-auto size-4 animate-spin"
              role="status"
              aria-label={t('common.loading')}
            />
          )}
        </div>
      </div>
    </aside>
  );
}
