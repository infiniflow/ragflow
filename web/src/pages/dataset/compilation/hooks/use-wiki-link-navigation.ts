import { useCallback, useState } from 'react';

export type NavEntry = {
  slug: string;
  title: string;
  pageType: string;
};

interface NavState {
  stack: NavEntry[];
  currentIndex: number;
}

export function useWikiLinkNavigation() {
  const [state, setState] = useState<NavState>({
    stack: [],
    currentIndex: -1,
  });

  const push = useCallback((entry: NavEntry) => {
    setState((prev) => ({
      stack: [...prev.stack.slice(0, prev.currentIndex + 1), entry],
      currentIndex: prev.currentIndex + 1,
    }));
  }, []);

  const goBack = useCallback(() => {
    setState((prev) => ({
      ...prev,
      currentIndex: Math.max(0, prev.currentIndex - 1),
    }));
  }, []);

  const reset = useCallback((entry: NavEntry) => {
    setState({ stack: [entry], currentIndex: 0 });
  }, []);

  const updateCurrentTitle = useCallback((title: string) => {
    setState((prev) => {
      if (prev.currentIndex < 0) return prev;
      const entry = prev.stack[prev.currentIndex];
      if (!entry || entry.title === title) return prev;
      const newStack = [...prev.stack];
      newStack[prev.currentIndex] = { ...entry, title };
      return { ...prev, stack: newStack };
    });
  }, []);

  const currentEntry =
    state.currentIndex >= 0 ? state.stack[state.currentIndex] : null;
  const previousEntry =
    state.currentIndex > 0 ? state.stack[state.currentIndex - 1] : null;
  const canGoBack = state.currentIndex > 0;

  return {
    currentEntry,
    previousEntry,
    canGoBack,
    push,
    goBack,
    reset,
    updateCurrentTitle,
  };
}
