import { useClearWiki } from '@/hooks/use-knowledge-request';
import { useCallback, useState } from 'react';

type UseWikiClearOptions = {
  onClearWiki?: () => void;
};

export function useWikiClear({ onClearWiki }: UseWikiClearOptions = {}) {
  const { clearWiki, loading } = useClearWiki();
  const [open, setOpen] = useState(false);

  const handleConfirm = useCallback(async () => {
    setOpen(false);
    const data = await clearWiki();
    if (data?.code === 0) {
      onClearWiki?.();
    }
  }, [clearWiki, onClearWiki]);

  return { open, setOpen, handleConfirm, loading };
}
