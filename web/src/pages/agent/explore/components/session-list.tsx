import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { useClientSearch } from '@/hooks/use-client-search';
import { IAgentLogResponse } from '@/interfaces/database/agent';
import { Plus } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useSelectDerivedSessionList } from '../hooks/use-select-derived-session-list';
import { SessionCard } from './session-card';

interface SessionListProps {
  selectedSessionId?: string;
  onSelectSession: (sessionId: string, isNew?: boolean) => void;
}

export function SessionList({
  selectedSessionId,
  onSelectSession,
}: SessionListProps) {
  const { t } = useTranslation();

  const { sessions, loading, addTemporarySession, removeTemporarySession } =
    useSelectDerivedSessionList();

  const { filteredData, handleSearchChange, searchKeyword } =
    useClientSearch<IAgentLogResponse>({
      data: sessions,
      searchFields: ['name'],
    });

  return (
    <section className="p-5 flex flex-col h-full">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2">
          <h2 className="text-base font-bold">{t('explore.sessions')}</h2>
          <span className="text-xs text-text-secondary">{sessions.length}</span>
        </div>
        <Button variant="ghost" size="icon" onClick={addTemporarySession}>
          <Plus className="h-4 w-4" />
        </Button>
      </div>
      <div className="mb-4">
        <SearchInput
          placeholder={t('explore.searchSessions')}
          onChange={handleSearchChange}
          value={searchKeyword}
        />
      </div>
      <div className="flex-1 overflow-auto space-y-3">
        {filteredData.map((session) => (
          <SessionCard
            key={session.id}
            session={session}
            selected={session.id === selectedSessionId}
            onClick={() => onSelectSession(session.id, session.is_new)}
            removeTemporarySession={removeTemporarySession}
          />
        ))}
        {!loading && filteredData.length === 0 && (
          <div className="text-center text-text-secondary py-8">
            {searchKeyword
              ? t('explore.noSessionsFound')
              : t('explore.noSessionsFound')}
          </div>
        )}
      </div>
    </section>
  );
}
