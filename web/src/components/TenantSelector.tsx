import React, { useState, useEffect } from 'react';
import { Modal, Select, Button, Avatar, Tag, Typography } from 'antd';
import { UserOutlined, PlusOutlined } from '@ant-design/icons';
import { useRequest } from 'umi';
import { getTenantList, switchTenant } from '@/services/tenant-service';

const { Text } = Typography;
const { Option } = Select;

interface TenantSelectorProps {
  value?: string;
  onChange?: (tenantId: string) => void;
  showCreate?: boolean;
  disabled?: boolean;
}

const TenantSelector: React.FC<TenantSelectorProps> = ({
  value,
  onChange,
  showCreate = true,
  disabled = false
}) => {
  const [modalVisible, setModalVisible] = useState(false);
  const [selectedTenant, setSelectedTenant] = useState<string>(value || '');
  
  // Fetch tenant list
  const { data: tenantData, loading, refresh } = useRequest(() => getTenantList(), {
    refreshDeps: [modalVisible]
  });
  
  const tenants = tenantData?.tenants || [];
  const currentTenant = tenants.find(t => t.id === (value || selectedTenant));
  
  const handleTenantChange = async (tenantId: string) => {
    setSelectedTenant(tenantId);
    if (onChange) {
      onChange(tenantId);
    }
    
    try {
      await switchTenant(tenantId);
      window.location.reload(); // Reload to apply tenant context
    } catch (error) {
      console.error('Failed to switch tenant:', error);
    }
  };
  
  const handleCreateTenant = () => {
    setModalVisible(true);
  };
  
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
      <Select
        value={selectedTenant}
        onChange={handleTenantChange}
        loading={loading}
        disabled={disabled}
        placeholder="Select tenant"
        style={{ minWidth: 150 }}
        dropdownRender={menu => (
          <div>
            {menu}
            {showCreate && (
              <div style={{ padding: 8, borderTop: '1px solid #f0f0f0' }}>
                <Button
                  type="text"
                  icon={<PlusOutlined />}
                  onClick={handleCreateTenant}
                  style={{ width: '100%' }}
                >
                  Create New Tenant
                </Button>
              </div>
            )}
          </div>
        )}
      >
        {tenants.map(tenant => (
          <Option key={tenant.id} value={tenant.id}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <Avatar size={20} icon={<UserOutlined />} />
              <span>{tenant.name}</span>
              <Tag color={tenant.role === 'owner' ? 'gold' : 'blue'}>
                {tenant.role}
              </Tag>
            </div>
          </Option>
        ))}
      </Select>
      
      {currentTenant && (
        <Text type="secondary" style={{ fontSize: 12 }}>
          Credits: {currentTenant.credit}
        </Text>
      )}
    </div>
  );
};

export default TenantSelector;