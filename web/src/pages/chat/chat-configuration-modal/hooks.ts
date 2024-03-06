import {
  useFetchTenantInfo,
  useSelectTenantInfo,
} from '@/hooks/userSettingHook';
import { useEffect } from 'react';

export const useFetchModelId = (visible: boolean) => {
  const fetchTenantInfo = useFetchTenantInfo(false);
  const tenantInfo = useSelectTenantInfo();

  useEffect(() => {
    if (visible) {
      fetchTenantInfo();
    }
  }, [visible, fetchTenantInfo]);

  return tenantInfo?.llm_id ?? '';
};
