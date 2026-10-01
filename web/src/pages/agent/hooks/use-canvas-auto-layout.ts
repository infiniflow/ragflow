import { useCallback, useEffect } from 'react';
import useGraphStore from '../store';
import { layoutCanvasNodes } from '../utils/canvas-auto-layout';

function isTypingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName;
  return (
    tag === 'INPUT' ||
    tag === 'TEXTAREA' ||
    tag === 'SELECT' ||
    target.isContentEditable
  );
}

export function useCanvasAutoLayout(): () => void {
  const arrange = useCallback(() => {
    const state = useGraphStore.getState();
    const updates = layoutCanvasNodes({
      nodes: state.nodes,
      edges: state.edges,
      selectedNodeIds: state.selectedNodeIds,
    });
    if (updates.size === 0) return;

    state.setNodes(
      state.nodes.map((node) => {
        const update = updates.get(node.id);
        if (!update) return node;
        if (update.width === undefined || update.height === undefined) {
          return {
            ...node,
            position: { x: update.x, y: update.y },
          };
        }
        return {
          ...node,
          position: { x: update.x, y: update.y },
          width: update.width,
          height: update.height,
          measured: {
            ...node.measured,
            width: update.width,
            height: update.height,
          },
        };
      }),
    );
  }, []);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.repeat || !event.shiftKey || !event.altKey) return;
      if (event.code !== 'KeyT') return;
      if (isTypingTarget(event.target)) return;
      event.preventDefault();
      arrange();
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [arrange]);

  return arrange;
}
