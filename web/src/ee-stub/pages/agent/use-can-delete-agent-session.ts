import { IAgentLogResponse } from '@/interfaces/database/agent';

export function useCanDeleteAgentSession(
  _session: IAgentLogResponse & { is_new?: boolean },
) {
  void _session;
  return true;
}
