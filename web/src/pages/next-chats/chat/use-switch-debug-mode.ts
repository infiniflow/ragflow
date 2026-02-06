import { useCallback, useState } from 'react';

export function useSwitchDebugMode() {
  const [isDebugMode, setIsDebugMode] = useState(false);

  const switchDebugMode = useCallback(() => {
    setIsDebugMode(!isDebugMode);
  }, [isDebugMode]);

  return {
    isDebugMode,
    switchDebugMode,
  };
}
