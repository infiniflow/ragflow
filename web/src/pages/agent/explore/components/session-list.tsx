import { ConversationList } from '@/components/conversation-list';
import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { useClientSearch } from '@/hooks/use-client-search';
import { IAgentLogResponse } from '@/interfaces/database/agent';
import { Loader2, Plus } from 'lucide-react';
import { ReactNode, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useSelectDerivedSessionList } from '../hooks/use-select-derived-session-list';
import { SessionDropdown } from './session-dropdown';

type AgentSessionListItem = IAgentLogResponse & { is_new?: boolean };

interface SessionListProps {
  selectedSessionId?: string;
  onSelectSession: (sessionId: string, isNew?: boolean) => void;
}

export function SessionList({
  selectedSessionId = '',
  onSelectSession,
}: SessionListProps) {
  const { t } = useTranslation();

  const {
    sessions,
    total,
    loading,
    loadingMore,
    handleScroll,
    addTemporarySession,
    removeTemporarySession,
  } = useSelectDerivedSessionList();

  const { filteredData, handleSearchChange, searchKeyword, hasActiveSearch } =
    useClientSearch<AgentSessionListItem>({
      data: sessions,
      searchFields: ['name'],
    });
  const sessionCount = hasActiveSearch ? filteredData.length : total;

  const getDisplayName = useCallback(
    (session: AgentSessionListItem) =>
      session.is_new ? t('explore.newSession') : session.name,
    [t],
  );

  const renderActions = useCallback(
    (session: AgentSessionListItem, trigger: ReactNode) => (
      <SessionDropdown
        session={session}
        removeTemporarySession={removeTemporarySession}
      >
        {trigger}
      </SessionDropdown>
    ),
    [removeTemporarySession],
  );

  return (
    <aside className="flex h-full min-h-0 w-[296px] shrink-0 flex-col border-r-0.5 border-border-button px-5 pb-5 pt-5">
      <Button
        variant="outline"
        size="lg"
        block
        onClick={addTemporarySession}
        data-testid="agent-explore-session-new"
      >
        <Plus />
        {t('chat.newConversation')}
      </Button>

      <div className="mb-4 flex items-center gap-3 pt-8">
        <h2 className="text-base">{t('explore.sessions')}</h2>
        <data className="text-xs text-text-secondary" value={sessionCount}>
          {sessionCount}
        </data>
      </div>

      <div className="pb-4">
        <SearchInput
          placeholder={t('explore.searchSessions')}
          onChange={handleSearchChange}
          value={searchKeyword}
        />
      </div>

      <div className="min-h-0 flex-1 overflow-auto" onScroll={handleScroll}>
        <ConversationList
          items={filteredData}
          label={t('explore.sessions')}
          selectedId={selectedSessionId}
          onSelect={onSelectSession}
          renderActions={renderActions}
          getDisplayName={getDisplayName}
          testIdPrefix="agent-explore-session"
        />
        {loadingMore && (
          <Loader2
            className="mx-auto size-4 animate-spin"
            role="status"
            aria-label={t('common.loading')}
          />
        )}
        {!loading && filteredData.length === 0 && (
          <div className="py-8 text-center text-text-secondary">
            {t('explore.noSessionsFound')}
          </div>
        )}
      </div>
    </aside>
  );
}
