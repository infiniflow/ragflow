import { useCallback, useState } from 'react';

export function useSwitchDebugMode() {
  const [isDebugMode, setIsDebugMode] = useState(false);

  const enterDebugMode = useCallback(() => {
    setIsDebugMode(true);
  }, []);

  const exitDebugMode = useCallback(() => {
    setIsDebugMode(false);
  }, []);

  return {
    isDebugMode,
    enterDebugMode,
    exitDebugMode,
  };
}
