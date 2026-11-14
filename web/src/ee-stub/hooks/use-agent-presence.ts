/**
 * Enterprise seam for the canvas presence tracking. The enterprise build
 * replaces this file with the presence requests; open source has no shared
 * canvas presence, so the presence is always empty and the actions are
 * no-ops.
 */

import type { ICanvasPresenceResponse } from '@/interfaces/database/agent';

const emptyPresence: ICanvasPresenceResponse = {
  canvas_id: '',
  online_user_count: 0,
  users: [],
};

export const useFetchCanvasPresence = () => ({
  data: emptyPresence,
  loading: false,
  refetch: async () => {},
});

export const useJoinCanvasPresence = (_canvasId?: string) => ({
  joinPresence: async (_payload: unknown) => {},
  loading: false,
});

export const useHeartbeatCanvasPresence = () => ({
  heartbeat: async (_payload: unknown) => null,
  loading: false,
  data: null,
});

export const useLeaveCanvasPresence = () => ({
  leavePresence: async (_payload: unknown) => {},
  loading: false,
});
