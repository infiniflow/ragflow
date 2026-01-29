import { useCallback } from 'react';
import { useSaveGraph } from './use-save-graph';

// Hook to save the graph when a form field loses focus.
// This ensures changes are persisted immediately without waiting for the debounce timer.
export const useSaveOnBlur = () => {
  const { saveGraph } = useSaveGraph(false);

  const handleSaveOnBlur = useCallback(() => {
    saveGraph();
  }, [saveGraph]);

  return { handleSaveOnBlur };
};
