import { useEffect, useRef } from 'react';
import useGraphStore from './store';

// History management class
export class HistoryManager {
  private history: { nodes: any[]; edges: any[] }[] = [];
  private currentIndex: number = -1;
  private readonly maxSize: number = 50; // Limit maximum number of history records
  private setNodes: (nodes: any[]) => void;
  private setEdges: (edges: any[]) => void;
  private lastSavedState: string = ''; // Used to compare if state has changed

  constructor(
    setNodes: (nodes: any[]) => void,
    setEdges: (edges: any[]) => void,
  ) {
    this.setNodes = setNodes;
    this.setEdges = setEdges;
  }

  // Compare if two states are equal
  private statesEqual(
    state1: { nodes: any[]; edges: any[] },
    state2: { nodes: any[]; edges: any[] },
  ): boolean {
    return JSON.stringify(state1) === JSON.stringify(state2);
  }

  push(nodes: any[], edges: any[]) {
    const currentState = {
      nodes: JSON.parse(JSON.stringify(nodes)),
      edges: JSON.parse(JSON.stringify(edges)),
    };

    // If state hasn't changed, don't save
    if (
      this.history.length > 0 &&
      this.statesEqual(currentState, this.history[this.currentIndex])
    ) {
      return;
    }

    // If current index is not at the end of history, remove subsequent states
    if (this.currentIndex < this.history.length - 1) {
      this.history.splice(this.currentIndex + 1);
    }

    // Add current state
    this.history.push(currentState);

    // Limit history record size
    if (this.history.length > this.maxSize) {
      this.history.shift();
      this.currentIndex = this.history.length - 1;
    } else {
      this.currentIndex = this.history.length - 1;
    }

    // Update last saved state
    this.lastSavedState = JSON.stringify(currentState);
  }

  undo() {
    if (this.canUndo()) {
      this.currentIndex--;
      const prevState = this.history[this.currentIndex];
      this.setNodes(JSON.parse(JSON.stringify(prevState.nodes)));
      this.setEdges(JSON.parse(JSON.stringify(prevState.edges)));
      return true;
    }
    return false;
  }

  redo() {
    console.log('redo');
    if (this.canRedo()) {
      this.currentIndex++;
      const nextState = this.history[this.currentIndex];
      this.setNodes(JSON.parse(JSON.stringify(nextState.nodes)));
      this.setEdges(JSON.parse(JSON.stringify(nextState.edges)));
      return true;
    }
    return false;
  }

  canUndo() {
    return this.currentIndex > 0;
  }

  canRedo() {
    return this.currentIndex < this.history.length - 1;
  }

  // Reset history records
  reset() {
    this.history = [];
    this.currentIndex = -1;
    this.lastSavedState = '';
  }
}

export const useAgentHistoryManager = () => {
  // Get current state and history state
  const nodes = useGraphStore((state) => state.nodes);
  const edges = useGraphStore((state) => state.edges);
  const setNodes = useGraphStore((state) => state.setNodes);
  const setEdges = useGraphStore((state) => state.setEdges);

  // Use useRef to keep HistoryManager instance unchanged
  const historyManagerRef = useRef<HistoryManager | null>(null);

  // Initialize HistoryManager
  if (!historyManagerRef.current) {
    historyManagerRef.current = new HistoryManager(setNodes, setEdges);
  }

  const historyManager = historyManagerRef.current;

  // Save state history - use useEffect instead of useMemo to avoid re-rendering
  useEffect(() => {
    historyManager.push(nodes, edges);
  }, [nodes, edges, historyManager]);

  // Keyboard event handling
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Check if focused on an input element
      const activeElement = document.activeElement;
      const isInputFocused =
        activeElement instanceof HTMLInputElement ||
        activeElement instanceof HTMLTextAreaElement ||
        activeElement?.hasAttribute('contenteditable');

      // Skip keyboard shortcuts if typing in an input field
      if (isInputFocused) {
        return;
      }
      // Ctrl+Z or Cmd+Z undo
      if (
        (e.ctrlKey || e.metaKey) &&
        (e.key === 'z' || e.key === 'Z') &&
        !e.shiftKey
      ) {
        e.preventDefault();
        historyManager.undo();
      }
      // Ctrl+Shift+Z or Cmd+Shift+Z redo
      else if (
        (e.ctrlKey || e.metaKey) &&
        (e.key === 'z' || e.key === 'Z') &&
        e.shiftKey
      ) {
        e.preventDefault();
        historyManager.redo();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [historyManager]);
};
