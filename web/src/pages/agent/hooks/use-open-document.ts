import { useCallback } from 'react';

export function useOpenDocument() {
  const openDocument = useCallback(() => {
    window.open(
      'https://ragflow.io/docs/dev/category/agent-components',
      '_blank',
    );
  }, []);

  return openDocument;
}
