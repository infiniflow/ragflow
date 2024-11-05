import axios from 'axios';
import { useCallback, useEffect, useState } from 'react';

export const useCatchDocumentError = (url: string) => {
  const [error, setError] = useState<string>('');

  const fetchDocument = useCallback(async () => {
    const { data } = await axios.get(url);
    if (data.code !== 0) {
      setError(data?.message);
    }
  }, [url]);
  useEffect(() => {
    fetchDocument();
  }, [fetchDocument]);

  return error;
};
