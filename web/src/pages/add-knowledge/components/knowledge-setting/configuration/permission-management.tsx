import React, { useState } from 'react';
import { Modal, Button, Avatar, List, Dropdown, Space, Tabs, Card } from 'antd';
import { 
  KeyOutlined, 
  UserOutlined, 
  TeamOutlined, 
  UsergroupAddOutlined, 
  SettingOutlined, 
  PlusOutlined,
  UserAddOutlined
} from '@ant-design/icons';
import { useTranslate } from '@/hooks/common-hooks';

// 协作者类型
interface Collaborator {
  id: string;
  name: string;
  avatar?: string;
  permission: 'read' | 'write' | 'admin';
}

// 权限管理组件属性
interface PermissionManagementProps {
  value?: Collaborator[];
  onChange?: (collaborators: Collaborator[]) => void;
  currentUser?: {
    id: string;
    name: string;
    avatar?: string;
  };
}

// 权限选项
const permissionOptions = [
  { key: 'read', label: '读取权限' },
  { key: 'write', label: '写入权限' },
  { key: 'admin', label: '管理权限' },
];

// 成员表格组件
const MemberTable = () => {
  const { t } = useTranslate('knowledgeConfiguration');
  // 模拟数据
  const mockMembers = [
    { id: '1', name: '用户1', avatar: 'https://joesch.moe/api/v1/random?1', permission: 'admin' },
    { id: '2', name: '用户2', avatar: 'https://joesch.moe/api/v1/random?2', permission: 'read' },
  ];

  return (
    <List
      itemLayout="horizontal"
      dataSource={mockMembers}
      renderItem={item => (
        <List.Item
          actions={[
            <Dropdown
              key="permission"
              menu={{
                items: permissionOptions,
                onClick: () => {},
              }}
              trigger={['click']}
            >
              <Button type="text">
                {item.permission === 'admin' ? '管理权限' : 
                 item.permission === 'write' ? '写入权限' : '读取权限'} 
                <span>▼</span>
              </Button>
            </Dropdown>,
            <Button key="delete" type="text" danger>
              移除
            </Button>,
          ]}
        >
          <List.Item.Meta
            avatar={<Avatar src={item.avatar}>{item.name.charAt(0)}</Avatar>}
            title={item.name}
          />
        </List.Item>
      )}
    />
  );
};

// 部门表格组件
const DepartmentTable = () => {
  const mockDepartments = [
    { id: 'd1', name: '研发部', permission: 'read' },
    { id: 'd2', name: '产品部', permission: 'read' },
  ];

  return (
    <List
      itemLayout="horizontal"
      dataSource={mockDepartments}
      renderItem={item => (
        <List.Item
          actions={[
            <Dropdown
              key="permission"
              menu={{
                items: permissionOptions,
                onClick: () => {},
              }}
              trigger={['click']}
            >
              <Button type="text">
                {item.permission === 'admin' ? '管理权限' : 
                 item.permission === 'write' ? '写入权限' : '读取权限'} 
                <span>▼</span>
              </Button>
            </Dropdown>,
            <Button key="delete" type="text" danger>
              移除
            </Button>,
          ]}
        >
          <List.Item.Meta
            avatar={<Avatar icon={<TeamOutlined />} />}
            title={item.name}
          />
        </List.Item>
      )}
    />
  );
};

// 群组表格组件
const GroupTable = () => {
  const mockGroups = [
    { id: 'g1', name: '项目A组', permission: 'read' },
    { id: 'g2', name: '技术讨论群', permission: 'read' },
  ];

  return (
    <List
      itemLayout="horizontal"
      dataSource={mockGroups}
      renderItem={item => (
        <List.Item
          actions={[
            <Dropdown
              key="permission"
              menu={{
                items: permissionOptions,
                onClick: () => {},
              }}
              trigger={['click']}
            >
              <Button type="text">
                {item.permission === 'admin' ? '管理权限' : 
                 item.permission === 'write' ? '写入权限' : '读取权限'} 
                <span>▼</span>
              </Button>
            </Dropdown>,
            <Button key="delete" type="text" danger>
              移除
            </Button>,
          ]}
        >
          <List.Item.Meta
            avatar={<Avatar icon={<UsergroupAddOutlined />} />}
            title={item.name}
          />
        </List.Item>
      )}
    />
  );
};

// 权限管理组件
const PermissionManagement: React.FC<PermissionManagementProps> = ({
  value = [],
  onChange,
  currentUser,
}) => {
  const { t } = useTranslate('knowledgeConfiguration');
  const [isModalVisible, setIsModalVisible] = useState(false);
  const [collaborators, setCollaborators] = useState<Collaborator[]>(value);
  const [activeTab, setActiveTab] = useState('members');
  const [currentModal, setCurrentModal] = useState<'add' | 'manage'>('add');

  // 打开添加协作者模态框
  const showAddModal = () => {
    setCurrentModal('add');
    setIsModalVisible(true);
  };

  // 处理模态框确认
  const handleOk = () => {
    setIsModalVisible(false);
    if (onChange) {
      onChange(collaborators);
    }
  };

  // 处理模态框取消
  const handleCancel = () => {
    setIsModalVisible(false);
  };

  // Tab项配置
  const tabItems = [
    {
      key: 'members',
      label: (
        <span>
          <UserOutlined />
          成员
        </span>
      ),
      children: <MemberTable />,
    },
    {
      key: 'departments',
      label: (
        <span>
          <TeamOutlined />
          部门
        </span>
      ),
      children: <DepartmentTable />,
    },
    {
      key: 'groups',
      label: (
        <span>
          <UsergroupAddOutlined />
          群组
        </span>
      ),
      children: <GroupTable />,
    },
  ];

  // 渲染添加协作者模态框
  const renderAddCollaboratorModal = () => {
    return (
      <Modal
        title="添加协作者"
        open={isModalVisible}
        onOk={handleOk}
        onCancel={handleCancel}
        footer={[
          <Button key="back" onClick={handleCancel}>
            取消
          </Button>,
          <Button key="submit" type="primary" onClick={handleOk}>
            保存
          </Button>,
        ]}
        width={700}
      >
        <Tabs 
          activeKey={activeTab}
          onChange={setActiveTab}
          items={tabItems} 
        />
      </Modal>
    );
  };

  // 获取展示用户名
  const getDisplayUserName = () => {
    return currentUser?.name || t('currentUser');
  };

  // 获取用户头像的第一个字符
  const getAvatarChar = () => {
    if (currentUser?.name && currentUser.name.length > 0) {
      return currentUser.name.charAt(0);
    }
    return t('user').charAt(0);
  };

  return (
    <div>
      <div style={{ 
        border: '1px solid #eee', 
        borderRadius: '8px', 
        padding: '20px'
      }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
          <div style={{ display: 'flex', alignItems: 'center' }}>
            <KeyOutlined style={{ marginRight: '8px' }} />
            <span style={{ fontWeight: 'bold' }}>权限管理</span>
          </div>
          <Button 
            type="primary" 
            icon={<UserAddOutlined />}
            onClick={showAddModal}
          >
            添加协作者
          </Button>
        </div>
        
        <Card style={{ marginBottom: '16px' }} bordered={false}>
          <Tabs 
            activeKey={activeTab}
            onChange={setActiveTab}
            items={tabItems}
          />
        </Card>
      </div>

      {renderAddCollaboratorModal()}
    </div>
  );
};

export default PermissionManagement;