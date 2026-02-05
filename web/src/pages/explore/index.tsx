import { useCallback } from 'react';
import { useNavigate } from 'react-router';
import { CanvasList } from './components/canvas-list';
import { SessionChat } from './components/session-chat';
import { SessionList } from './components/session-list';
import { useExploreUrlParams } from './hooks/use-explore-url-params';

export default function Explore() {
  const { canvasId, sessionId, setSessionId } = useExploreUrlParams();
  const navigate = useNavigate();

  const handleCanvasSelect = useCallback(
    (id: string) => {
      navigate(`/explore/${id}`);
    },
    [navigate],
  );

  const handleSessionSelect = useCallback(
    (id: string, isNew?: boolean) => {
      setSessionId(id, isNew);
    },
    [setSessionId],
  );

  return (
    <section className="flex h-full">
      <div className="w-[280px] border-r min-w-0">
        <CanvasList
          selectedCanvasId={canvasId}
          onSelectCanvas={handleCanvasSelect}
        />
      </div>

      <div className="w-[296px] border-r min-w-0">
        <SessionList
          selectedSessionId={sessionId}
          onSelectSession={handleSessionSelect}
        />
      </div>

      <div className="flex-1 min-w-0">
        <SessionChat canvasId={canvasId || ''} sessionId={sessionId} />
      </div>
    </section>
  );
}
