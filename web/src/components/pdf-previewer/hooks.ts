import { Authorization } from '@/constants/authorization';
import { getAuthorization } from '@/utils/authorization-util';
import axios from 'axios';
import { useCallback, useEffect, useMemo, useState } from 'react';

export const useCatchDocumentError = (url: string) => {
  const httpHeaders = useMemo(() => {
    return {
      [Authorization]: getAuthorization(),
    };
  }, []);
  const [error, setError] = useState<string>('');

  const fetchDocument = useCallback(async () => {
    const { data } = await axios.get(url, { headers: httpHeaders });
    if (data.code !== 0) {
      setError(data?.message);
    }
  }, [url, httpHeaders]);
  useEffect(() => {
    fetchDocument();
  }, [fetchDocument]);

  return error;
};
