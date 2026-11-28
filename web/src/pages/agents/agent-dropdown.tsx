import {
  ConfirmDeleteDialog,
  ConfirmDeleteDialogNode,
} from '@/components/confirm-delete-dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  AgentDropdownGuard,
  shouldShowAgentDropdown,
} from '@/ee/pages/agents/agent-dropdown-guard';
import { useDeleteAgent } from '@/hooks/use-agent-request';
import { IFlow } from '@/interfaces/database/agent';
import { PenLine, Tag, Trash2 } from 'lucide-react';
import {
  MouseEventHandler,
  PropsWithChildren,
  useCallback,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import { AgentTagEditor } from './agent-tag-editor';
import { useRenameAgent } from './use-rename-agent';

export function AgentDropdown({
  children,
  showAgentRenameModal,
  agent,
  showPrivilegeModal,
}: PropsWithChildren &
  Pick<ReturnType<typeof useRenameAgent>, 'showAgentRenameModal'> & {
    agent: IFlow;
    showPrivilegeModal(): void;
  }) {
  const { t } = useTranslation();
  const { deleteAgent } = useDeleteAgent();
  const [tagEditorOpen, setTagEditorOpen] = useState(false);

  const handleShowAgentRenameModal: MouseEventHandler<HTMLDivElement> =
    useCallback(
      (e) => {
        e.stopPropagation();
        showAgentRenameModal(agent);
      },
      [agent, showAgentRenameModal],
    );

  const handleEditTags: MouseEventHandler<HTMLDivElement> = useCallback((e) => {
    e.stopPropagation();
    setTagEditorOpen(true);
  }, []);

  const handleDelete: MouseEventHandler<HTMLDivElement> = useCallback(() => {
    deleteAgent(agent.id);
  }, [agent.id, deleteAgent]);

  if (!shouldShowAgentDropdown(agent)) {
    return null;
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>{children}</DropdownMenuTrigger>
        <DropdownMenuContent>
          <AgentDropdownGuard
            agent={agent}
            showPrivilegeModal={showPrivilegeModal}
            manageItems={
              <>
                <DropdownMenuItem
                  justifyBetween={false}
                  onClick={handleShowAgentRenameModal}
                >
                  <PenLine /> {t('common.rename')}
                </DropdownMenuItem>
                <DropdownMenuItem
                  justifyBetween={false}
                  onClick={handleEditTags}
                >
                  <Tag /> {t('flow.editTags')}
                </DropdownMenuItem>
              </>
            }
            deleteItems={
              <>
                <DropdownMenuSeparator />
                <ConfirmDeleteDialog
                  onOk={handleDelete}
                  title={t('deleteModal.delAgent')}
                  content={{
                    node: (
                      <ConfirmDeleteDialogNode
                        avatar={{ avatar: agent.avatar, name: agent.title }}
                        name={agent.title}
                      />
                    ),
                  }}
                >
                  <DropdownMenuItem
                    justifyBetween={false}
                    className="text-state-error"
                    onSelect={(e) => {
                      e.preventDefault();
                    }}
                    onClick={(e) => {
                      e.stopPropagation();
                    }}
                  >
                    <Trash2 /> {t('common.delete')}
                  </DropdownMenuItem>
                </ConfirmDeleteDialog>
              </>
            }
          />
        </DropdownMenuContent>
      </DropdownMenu>
      <AgentTagEditor
        agent={agent}
        open={tagEditorOpen}
        onOpenChange={setTagEditorOpen}
      />
    </>
  );
}
