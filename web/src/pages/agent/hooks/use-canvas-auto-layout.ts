import { useStore } from '@xyflow/react';
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

export function useCanvasAutoLayout(enabled = true): () => void {
  const nodesDraggable = useStore((state) => state.nodesDraggable);
  const interactive = enabled && nodesDraggable;

  const arrange = useCallback(() => {
    if (!interactive) return;
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
  }, [interactive]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (!interactive) return;
      if (event.repeat || event.ctrlKey || event.metaKey) return;
      if (!event.shiftKey || !event.altKey || event.code !== 'KeyT') return;
      if (isTypingTarget(event.target)) return;
      if (
        event.target instanceof Element &&
        event.target.closest('[role="dialog"], [role="alertdialog"]')
      ) {
        return;
      }
      event.preventDefault();
      arrange();
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [arrange, interactive]);

  return arrange;
}
