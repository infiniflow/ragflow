import { IDialog } from '@/interfaces/database/chat';

export function useChatSessionAccess(
  _chat: Pick<IDialog, 'tenant_id' | 'operator_permission'> | undefined,
  _currentUserId: string | undefined,
  _sessionOwnerId?: string,
  _isNew = false,
) {
  return {
    canRead: true,
    canCreateSession: true,
    canAccessChatSettings: true,
    canWriteAll: true,
    canManageAll: true,
    canSend: true,
    canDeleteOwnSession: true,
    canDeleteMessages: true,
    canFeedback: true,
  };
}
