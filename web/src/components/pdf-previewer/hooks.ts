import axios from 'axios';
import { useCallback, useEffect, useState } from 'react';

export const useCatchDocumentError = (url: string) => {
  const [error, setError] = useState<string>('');

  const fetchDocument = useCallback(async () => {
    if (url.endsWith('undefined') || url.endsWith('/get/')) {
      return;
    }
    try {
      const ret = await axios.get(url, { responseType: 'arraybuffer' });
      const data = ret.data;
      if (data instanceof ArrayBuffer) {
        try {
          const jsonData = JSON.parse(new TextDecoder().decode(data));
          if (jsonData.code !== 0) {
            setError(jsonData?.message);
          }
        } catch (e) {
          //
        }
      }
    } catch (e) {
      console.error(e);
    }
  }, [url]);
  useEffect(() => {
    fetchDocument();
  }, [fetchDocument]);

  return error;
};
