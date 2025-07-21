import React, { createContext, useContext, useState, useEffect, ReactNode } from 'react';
import { message } from 'antd';
import { useRequest } from 'umi';
import { getTenantList, switchTenant, getTenantConfig } from '@/services/tenant-service';

export interface Tenant {
  id: string;
  name: string;
  description?: string;
  credit: number;
  role?: string;
  status?: string;
  create_time?: string;
  update_time?: string;
}

interface TenantContextType {
  currentTenant: Tenant | null;
  tenantList: Tenant[];
  loading: boolean;
  switchTenant: (tenantId: string) => Promise<void>;
  refreshTenants: () => void;
  isOwner: boolean;
  isNormalUser: boolean;
  hasRole: (role: string) => boolean;
}

const TenantContext = createContext<TenantContextType>({
  currentTenant: null,
  tenantList: [],
  loading: false,
  switchTenant: async () => {},
  refreshTenants: () => {},
  isOwner: false,
  isNormalUser: false,
  hasRole: () => false,
});

export const useTenant = () => useContext(TenantContext);

interface TenantProviderProps {
  children: ReactNode;
}

export const TenantProvider: React.FC<TenantProviderProps> = ({ children }) => {
  const [currentTenant, setCurrentTenant] = useState<Tenant | null>(null);
  const [tenantList, setTenantList] = useState<Tenant[]>([]);

  const { data: tenantsData, loading, refresh } = useRequest(() => getTenantList(), {
    refreshDeps: [],
    onSuccess: (data) => {
      if (data?.tenants) {
        setTenantList(data.tenants);
        // Set the first tenant as current if none is selected
        if (!currentTenant && data.tenants.length > 0) {
          setCurrentTenant(data.tenants[0]);
        }
      }
    },
    onError: (error) => {
      console.error('Failed to load tenants:', error);
      message.error('Failed to load tenant information');
    },
  });

  const handleSwitchTenant = async (tenantId: string) => {
    try {
      await switchTenant(tenantId);
      const tenant = tenantList.find(t => t.id === tenantId);
      if (tenant) {
        setCurrentTenant(tenant);
        message.success(`Switched to ${tenant.name}`);
        // Reload the page to apply tenant context
        window.location.reload();
      }
    } catch (error) {
      console.error('Failed to switch tenant:', error);
      message.error('Failed to switch tenant');
    }
  };

  const hasRole = (role: string) => {
    return currentTenant?.role === role;
  };

  const isOwner = hasRole('owner');
  const isNormalUser = hasRole('normal') || isOwner;

  const value: TenantContextType = {
    currentTenant,
    tenantList,
    loading,
    switchTenant: handleSwitchTenant,
    refreshTenants: refresh,
    isOwner,
    isNormalUser,
    hasRole,
  };

  return (
    <TenantContext.Provider value={value}>
      {children}
    </TenantContext.Provider>
  );
};