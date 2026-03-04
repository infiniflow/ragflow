import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { MoreButton } from '@/components/more-button';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { SearchInput } from '@/components/ui/input';
import { useSetModalState } from '@/hooks/common-hooks';
import {
  useFetchDialog,
  useGetChatSearchParams,
  useRemoveConversation,
} from '@/hooks/use-chat-request';
import {
  LucideCopyX,
  LucideListChecks,
  LucidePanelLeftClose,
  LucidePlus,
  LucideTrash2,
  LucideUndo2,
} from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
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
  const { data } = useFetchDialog();
  const { visible, switchVisible } = useSetModalState(true);
  const { removeConversation } = useRemoveConversation();

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
    if (selectedIds.size > 0) {
      await removeConversation(Array.from(selectedIds));
      exitSelectionMode();
    }
  }, [selectedIds, removeConversation, exitSelectionMode]);

  const selectedCount = useMemo(() => selectedIds.size, [selectedIds]);
  const { conversationId } = useGetChatSearchParams();

  if (!visible) {
    return (
      <div className="p-5">
        <Button
          variant="transparent"
          size="icon-sm"
          className="border-0"
          onClick={switchVisible}
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
    <aside className="p-5 w-[296px] flex flex-col" role="complementary">
      <header className="flex items-center text-base justify-between gap-2">
        <div className="flex gap-3 items-center min-w-0">
          <RAGFlowAvatar
            avatar={data.icon}
            name={data.name}
            className="size-8"
          />

          <span className="flex-1 truncate">{data.name}</span>
        </div>

        <Button
          variant="transparent"
          size="icon-sm"
          className="border-0"
          onClick={switchVisible}
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
            <Button variant="ghost" size="icon-xs" onClick={exitSelectionMode}>
              <LucideUndo2 size={16} />
            </Button>
          ) : (
            // New conversation
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={addTemporaryConversation}
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
            >
              <Button variant="delete" size="icon-xs">
                <LucideTrash2 />
              </Button>
            </ConfirmDeleteDialog>
          ) : (
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={selectionMode ? toggleSelectAll : toggleSelectionMode}
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
              >
                <label className="flex items-center gap-2">
                  <Checkbox
                    checked={selectedIds.has(x.id)}
                    onCheckedChange={() => toggleSelection(x.id)}
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
                  >
                    {x.name}
                  </button>

                  <ConversationDropdown
                    conversation={x}
                    removeTemporaryConversation={removeTemporaryConversation}
                  >
                    <MoreButton></MoreButton>
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
