import { SearchInput } from '@/components/ui/input';
import { useFetchSessionsByCanvasId } from '@/hooks/use-agent-request';
import { useClientSearch } from '@/hooks/use-client-search';
import { IAgentLogResponse } from '@/interfaces/database/agent';
import { useTranslation } from 'react-i18next';
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

  const { data: sessions, loading } = useFetchSessionsByCanvasId();

  const { filteredData, handleSearchChange, searchKeyword } =
    useClientSearch<IAgentLogResponse>({
      data: sessions,
      searchFields: [
        // Search in user message contents
        (item) =>
          item.message
            .filter((msg) => msg.role === 'user')
            .map((msg) => msg.content)
            .join(' '),
        // Search in session ID
        'id' as keyof IAgentLogResponse,
      ],
    });

  return (
    <section className="p-5 flex flex-col h-full">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-base font-bold">{t('explore.sessions')}</h2>
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
            onClick={() => onSelectSession(session.id)}
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
