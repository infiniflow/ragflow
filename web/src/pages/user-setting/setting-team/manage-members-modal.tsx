import React, { useState } from 'react';
import { Modal, Input, Button, Checkbox, Row, Col } from 'antd';
import { useTranslation } from 'react-i18next';
import { SearchOutlined } from '@ant-design/icons';

interface MemberItem {
  id: string;
  name: string;
  isOwner?: boolean;
}

interface ManageMembersModalProps {
  visible: boolean;
  title: string;
  members: MemberItem[];
  onOk: (selectedMembers: string[]) => void;
  onCancel: () => void;
}

const ManageMembersModal: React.FC<ManageMembersModalProps> = ({
  visible,
  title,
  members,
  onOk,
  onCancel,
}) => {
  const { t } = useTranslation();
  const [selectedMembers, setSelectedMembers] = useState<string[]>([]);
  const [leftSearchValue, setLeftSearchValue] = useState('');
  const [rightSearchValue, setRightSearchValue] = useState('');

  // 模拟已添加的成员
  const addedMembers = members.filter(m => m.id === '1');
  const availableMembers = members.filter(m => m.id !== '1');

  const handleTransfer = () => {
    // 处理成员转移
  };

  const handleSave = () => {
    onOk(selectedMembers);
  };

  return (
    <Modal
      title={title}
      open={visible}
      onCancel={onCancel}
      width={800}
      footer={[
        <Button key="cancel" onClick={onCancel}>
          取消
        </Button>,
        <Button key="submit" type="primary" onClick={handleSave}>
          保存
        </Button>,
      ]}
    >
      <div style={{ display: 'flex', height: '400px' }}>
        <div style={{ flex: 1, padding: '8px', border: '1px solid #e8e8e8', borderRadius: '4px', marginRight: '8px' }}>
          <Input
            placeholder="Search"
            prefix={<SearchOutlined />}
            value={leftSearchValue}
            onChange={(e) => setLeftSearchValue(e.target.value)}
            style={{ marginBottom: '8px' }}
          />
          <div style={{ height: '350px', overflowY: 'auto' }}>
            {availableMembers.map((item) => (
              <div key={item.id} style={{ padding: '8px 0', borderBottom: '1px solid #f0f0f0' }}>
                <Checkbox
                  onChange={(e) => {
                    if (e.target.checked) {
                      setSelectedMembers([...selectedMembers, item.id]);
                    } else {
                      setSelectedMembers(selectedMembers.filter(id => id !== item.id));
                    }
                  }}
                >
                  {item.name}
                </Checkbox>
              </div>
            ))}
          </div>
        </div>

        <Button
          type="primary"
          style={{ alignSelf: 'center', margin: '0 8px' }}
          onClick={handleTransfer}
          icon={<span>&gt;</span>}
        />

        <div style={{ flex: 1, padding: '8px', border: '1px solid #e8e8e8', borderRadius: '4px', marginLeft: '8px' }}>
          <Input
            placeholder="Search"
            prefix={<SearchOutlined />}
            value={rightSearchValue}
            onChange={(e) => setRightSearchValue(e.target.value)}
            style={{ marginBottom: '8px' }}
          />
          <div style={{ height: '350px', overflowY: 'auto' }}>
            {addedMembers.map((item) => (
              <div key={item.id} style={{ padding: '8px 0', borderBottom: '1px solid #f0f0f0', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <div>{item.name} {item.isOwner && <span style={{ background: '#1677ff', color: 'white', padding: '0 4px', borderRadius: '2px', fontSize: '12px' }}>所有者</span>}</div>
                <Checkbox />
              </div>
            ))}
          </div>
        </div>
      </div>
    </Modal>
  );
};

export default ManageMembersModal;
