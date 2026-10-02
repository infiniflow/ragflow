import { create } from 'zustand';

export type CanvasEdgeRouting = 'bezier' | 'orthogonal';

const storageKey = 'ragflow-canvas-edge-route';

function readRoute(): CanvasEdgeRouting {
  if (typeof window === 'undefined') return 'bezier';
  try {
    return window.localStorage.getItem(storageKey) === 'orthogonal'
      ? 'orthogonal'
      : 'bezier';
  } catch {
    return 'bezier';
  }
}

type EdgeRouteState = {
  route: CanvasEdgeRouting;
  setRoute: (route: CanvasEdgeRouting) => void;
};

export const useCanvasEdgeRoute = create<EdgeRouteState>((set) => ({
  route: readRoute(),
  setRoute: (route) => {
    try {
      window.localStorage.setItem(storageKey, route);
    } catch {
      // The canvas still switches for this session when storage is blocked.
    }
    set({ route });
  },
}));
