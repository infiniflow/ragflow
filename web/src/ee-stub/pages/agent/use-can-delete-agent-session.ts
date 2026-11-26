import { IAgentLogResponse } from '@/interfaces/database/agent';

export function useCanDeleteAgentSession(
  _session: IAgentLogResponse & { is_new?: boolean },
) {
  return true;
}
