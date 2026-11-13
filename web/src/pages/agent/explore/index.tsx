import { useFetchSessionManually } from '@/hooks/use-agent-request';
import { useCallback } from 'react';
import { SessionChat } from './components/session-chat';
import { SessionList } from './components/session-list';
import { useExploreUrlParams } from './hooks/use-explore-url-params';

export default function AgentExplore() {
  const { sessionId, setSessionId } = useExploreUrlParams();
  const { fetchSessionManually, data: session } = useFetchSessionManually();

  const handleSessionSelect = useCallback(
    (id: string, isNew?: boolean) => {
      setSessionId(id, isNew);
      fetchSessionManually(id);
    },
    [fetchSessionManually, setSessionId],
  );

  return (
    <section className="flex h-full min-h-0">
      <div className="w-[296px] min-w-0 border-r">
        <SessionList
          selectedSessionId={sessionId}
          onSelectSession={handleSessionSelect}
        />
      </div>

      <div className="min-w-0 flex-1">
        <SessionChat session={session} />
      </div>
    </section>
  );
}
