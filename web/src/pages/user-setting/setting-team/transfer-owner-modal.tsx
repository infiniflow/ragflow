import React, { useState } from 'react';
import { Modal, Select, Switch, Avatar } from 'antd';
import { useTranslation } from 'react-i18next';

interface TransferOwnerModalProps {
  visible: boolean;
  currentOwner: {
    id: string;
    name: string;
    avatar: string;
  };
  onOk: (newOwnerId: string, keepAdminPermissions: boolean) => void;
  onCancel: () => void;
}

const TransferOwnerModal: React.FC<TransferOwnerModalProps> = ({
  visible,
  currentOwner,
  onOk,
  onCancel,
}) => {
  const { t } = useTranslation();
  const [selectedUserId, setSelectedUserId] = useState<string>('');
  const [keepAdminPermissions, setKeepAdminPermissions] = useState(true);

  // 模拟用户列表
  const userOptions = [
    { value: '1', label: 'User 1' },
    { value: '2', label: 'User 2' },
    { value: '3', label: 'User 3' },
  ];

  const handleOk = () => {
    if (selectedUserId) {
      onOk(selectedUserId, keepAdminPermissions);
    }
  };

  return (
    <Modal
      title="转移所有者"
      open={visible}
      onOk={handleOk}
      onCancel={onCancel}
      okText="保存"
      cancelText="取消"
    >
      <div style={{ marginBottom: '20px' }}>
        <div style={{ display: 'flex', alignItems: 'center', marginBottom: '10px' }}>
          <Avatar src={currentOwner.avatar} size={40} />
          <span style={{ marginLeft: '10px' }}>{currentOwner.name}</span>
        </div>
      </div>
      <div style={{ marginBottom: '20px' }}>
        <div>转移给</div>
        <Select
          style={{ width: '100%' }}
          placeholder="请选择新所有者"
          options={userOptions}
          onChange={(value) => setSelectedUserId(value)}
        />
      </div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <span>保留管理员权限</span>
        <Switch checked={keepAdminPermissions} onChange={setKeepAdminPermissions} />
      </div>
    </Modal>
  );
};

export default TransferOwnerModal;
