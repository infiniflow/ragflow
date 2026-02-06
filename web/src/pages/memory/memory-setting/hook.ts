import { useUpdateMemory } from '@/pages/memories/hooks';
import { IMemory, IMemoryAppDetailProps } from '@/pages/memories/interface';
import { omit } from 'lodash';
import { useCallback, useState } from 'react';

export const useUpdateMemoryConfig = () => {
  const { updateMemory } = useUpdateMemory();
  const [loading, setLoading] = useState(false);
  const onMemoryRenameOk = useCallback(
    async (data: IMemory) => {
      let res;
      setLoading(true);
      if (data?.id) {
        // console.log('memory-->', memory, data);
        try {
          const params = omit(data, [
            'id',
            // 'memory_type',
            // 'embd_id',
            // 'storage_type',
          ]);
          res = await updateMemory({
            // ...memoryDataTemp,
            // data: data,
            id: data.id,
            ...params,
          } as unknown as IMemoryAppDetailProps);
          // if (res && res.data.code === 0) {
          //   message.success(t('message.update_success'));
          // } else {
          //   message.error(t('message.update_fail'));
          // }
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
