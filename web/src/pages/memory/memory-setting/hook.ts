import { useUpdateMemory } from '@/pages/memories/hooks';
import { IMemory, IMemoryAppDetailProps } from '@/pages/memories/interface';
import { omit } from 'lodash';
import { useCallback, useState } from 'react';

export const useUpdateMemoryConfig = () => {
  const { updateMemory } = useUpdateMemory();
  const [loading, setLoading] = useState(false);
  const onMemoryRenameOk = useCallback(
    async (data: IMemory) => {
      setLoading(true);
      if (data?.id) {
        // console.log('memory-->', memory, data);
        try {
          const params = omit(data, 'id');
          await updateMemory({
            id: data.id,
            ...params,
          } as unknown as IMemoryAppDetailProps);
        } catch (e) {
          console.error('error', e);
        }
      }
      setLoading(false);
    },
    [updateMemory],
  );
  return { onMemoryRenameOk, loading };
};
