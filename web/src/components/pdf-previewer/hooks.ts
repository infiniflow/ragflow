import axios from 'axios';
import { useCallback, useEffect, useState } from 'react';

export const useCatchDocumentError = (url: string) => {
  const [error, setError] = useState<string>('');

  const fetchDocument = useCallback(async () => {
    const { data } = await axios.get(url);
    if (data.retcode !== 0) {
      setError(data?.retmsg);
    }
  }, [url]);
  useEffect(() => {
    fetchDocument();
  }, [fetchDocument]);

  return error;
};
