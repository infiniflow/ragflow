import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useDeleteAgentSession } from '@/hooks/use-agent-request';
import { IAgentLogResponse } from '@/interfaces/database/agent';
import { Trash2 } from 'lucide-react';
import { MouseEventHandler, PropsWithChildren, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useExploreUrlParams } from '../hooks/use-explore-url-params';

interface SessionDropdownProps {
  session: IAgentLogResponse & { is_new?: boolean };
  removeTemporarySession?: (sessionId: string) => void;
}

export function SessionDropdown({
  children,
  session,
  removeTemporarySession,
}: PropsWithChildren<SessionDropdownProps>) {
  const { t } = useTranslation();
  const { canvasId, setSessionId, sessionId } = useExploreUrlParams();
  const { deleteAgentSession } = useDeleteAgentSession();

  const handleDelete: MouseEventHandler<HTMLDivElement> =
    useCallback(async () => {
      if (session.is_new && removeTemporarySession) {
        removeTemporarySession(session.id);
      } else if (canvasId) {
        const code = await deleteAgentSession({
          canvasId,
          sessionId: session.id,
        });
        if (code === 0 && sessionId === session.id) {
          setSessionId('', true);
        }
      }
    }, [
      session.is_new,
      session.id,
      removeTemporarySession,
      canvasId,
      deleteAgentSession,
      sessionId,
      setSessionId,
    ]);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>{children}</DropdownMenuTrigger>
      <DropdownMenuContent>
        <ConfirmDeleteDialog onOk={handleDelete}>
          <DropdownMenuItem
            className="text-state-error"
            onSelect={(e) => {
              e.preventDefault();
            }}
            onClick={(e) => {
              e.stopPropagation();
            }}
          >
            {t('common.delete')} <Trash2 />
          </DropdownMenuItem>
        </ConfirmDeleteDialog>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
