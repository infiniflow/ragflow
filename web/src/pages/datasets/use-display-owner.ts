import { useFetchTenantInfo } from '@/hooks/user-setting-hooks';
import { useCallback } from 'react';

export function useDisplayOwnerName() {
  const { data } = useFetchTenantInfo();
  const getOwnerName = useCallback(
    (tenantId: string, nickname: string) => {
      if (tenantId === data.tenant_id) {
        return null;
      }
      return nickname;
    },
    [data.tenant_id],
  );

  return getOwnerName;
}
