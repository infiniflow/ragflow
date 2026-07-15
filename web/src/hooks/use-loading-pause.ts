import { useEffect, useState } from 'react';

export function useLoadingPause(
  loading: boolean,
  content: string,
  delay = 600,
) {
  const [show, setShow] = useState(false);

  useEffect(() => {
    if (!loading || !content) {
      setShow(false);
      return;
    }

    const timer = setTimeout(() => {
      setShow(true);
    }, delay);

    return () => clearTimeout(timer);
  }, [loading, content, delay]);

  return show;
}
