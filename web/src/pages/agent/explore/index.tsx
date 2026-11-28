import { PageFrame } from '@/components/page-frame';
import { useFetchSessionManually } from '@/hooks/use-agent-request';
import { useCallback, useEffect } from 'react';
import { SessionChat } from './components/session-chat';
import { SessionList } from './components/session-list';
import { useExploreUrlParams } from './hooks/use-explore-url-params';

export default function AgentExplore() {
  const { sessionId, setSessionId } = useExploreUrlParams();
  const { fetchSessionManually, data: session } = useFetchSessionManually();

  useEffect(() => {
    if (!sessionId || session?.id === sessionId) {
      return;
    }

    void fetchSessionManually(sessionId).catch((error) => {
      console.error('Failed to load agent session:', error);
    });
  }, [fetchSessionManually, session?.id, sessionId]);

  const handleSessionSelect = useCallback(
    (id: string, isNew?: boolean) => {
      setSessionId(id, isNew);
    },
    [setSessionId],
  );

  return (
    <PageFrame>
      <section className="flex min-h-0 flex-1">
        <SessionList
          selectedSessionId={sessionId}
          onSelectSession={handleSessionSelect}
        />

        <div className="min-h-0 min-w-0 flex-1">
          <SessionChat session={session} />
        </div>
      </section>
    </PageFrame>
  );
}
