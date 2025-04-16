import React, { useState } from 'react';
import { Modal, Button, Space, List, Avatar, Tabs } from 'antd';
import { UserOutlined, TeamOutlined, UsergroupAddOutlined } from '@ant-design/icons';
import { useTranslate } from '@/hooks/common-hooks';

// 协作者类型定义
interface Collaborator {
  id: string;
  name: string;
  avatar?: string;
  type: 'member' | 'department' | 'group';
}

// 选项卡项
type TabItem = {
  key: string;
  label: React.ReactNode;
  children: React.ReactNode;
};

// 协作者权限枚举
export enum CollaboratorPermission {
  ReadOnly = 'readonly',
  ReadWrite = 'readwrite',
  Admin = 'admin',
}

// 协作者权限管理组件
interface CollaboratorManagementProps {
  value?: Collaborator[];
  onChange?: (collaborators: Collaborator[]) => void;
  defaultPermission?: CollaboratorPermission;
}

const CollaboratorManagement: React.FC<CollaboratorManagementProps> = ({
  value = [],
  onChange,
  defaultPermission = CollaboratorPermission.ReadOnly,
}) => {
  const { t } = useTranslate('knowledgeConfiguration');
  const [isModalVisible, setIsModalVisible] = useState(false);
  const [collaborators, setCollaborators] = useState<Collaborator[]>(value);
  const [activeTab, setActiveTab] = useState('member');

  // 模拟数据
  const mockMembers: Collaborator[] = [
    { id: '1', name: 'zyl', type: 'member', avatar: 'https://joesch.moe/api/v1/random?1' },
    { id: '2', name: '张三', type: 'member', avatar: 'https://joesch.moe/api/v1/random?2' },
    { id: '3', name: '李四', type: 'member', avatar: 'https://joesch.moe/api/v1/random?3' },
  ];

  const mockDepartments: Collaborator[] = [
    { id: 'd1', name: '研发部', type: 'department' },
    { id: 'd2', name: '产品部', type: 'department' },
    { id: 'd3', name: '市场部', type: 'department' },
  ];

  const mockGroups: Collaborator[] = [
    { id: 'g1', name: '项目A组', type: 'group' },
    { id: 'g2', name: '项目B组', type: 'group' },
    { id: 'g3', name: '技术讨论群', type: 'group' },
  ];

  // 显示模态框
  const showModal = () => {
    setIsModalVisible(true);
  };

  // 处理确定按钮
  const handleOk = () => {
    setIsModalVisible(false);
    if (onChange) {
      onChange(collaborators);
    }
  };

  // 处理取消按钮
  const handleCancel = () => {
    setIsModalVisible(false);
  };

  // 添加协作者
  const addCollaborator = (collaborator: Collaborator) => {
    // 如果已存在则不添加
    if (collaborators.find(item => item.id === collaborator.id && item.type === collaborator.type)) {
      return;
    }
    
    const newCollaborators = [...collaborators, collaborator];
    setCollaborators(newCollaborators);
    if (onChange) {
      onChange(newCollaborators);
    }
  };

  // 移除协作者
  const removeCollaborator = (id: string, type: string) => {
    const newCollaborators = collaborators.filter(
      item => !(item.id === id && item.type === type)
    );
    setCollaborators(newCollaborators);
    if (onChange) {
      onChange(newCollaborators);
    }
  };

  // 获取选项卡项
  const getTabItems = (): TabItem[] => {
    return [
      {
        key: 'member',
        label: (
          <span>
            <UserOutlined />
            {t('member')}
          </span>
        ),
        children: (
          <List
            itemLayout="horizontal"
            dataSource={mockMembers}
            renderItem={item => (
              <List.Item
                actions={[
                  <Button
                    type="link"
                    onClick={() => addCollaborator(item)}
                  >
                    {t('add')}
                  </Button>,
                ]}
              >
                <List.Item.Meta
                  avatar={<Avatar src={item.avatar} />}
                  title={item.name}
                />
              </List.Item>
            )}
          />
        ),
      },
      {
        key: 'department',
        label: (
          <span>
            <TeamOutlined />
            {t('department')}
          </span>
        ),
        children: (
          <List
            itemLayout="horizontal"
            dataSource={mockDepartments}
            renderItem={item => (
              <List.Item
                actions={[
                  <Button
                    type="link"
                    onClick={() => addCollaborator(item)}
                  >
                    {t('add')}
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
        ),
      },
      {
        key: 'group',
        label: (
          <span>
            <UsergroupAddOutlined />
            {t('group')}
          </span>
        ),
        children: (
          <List
            itemLayout="horizontal"
            dataSource={mockGroups}
            renderItem={item => (
              <List.Item
                actions={[
                  <Button
                    type="link"
                    onClick={() => addCollaborator(item)}
                  >
                    {t('add')}
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
        ),
      },
    ];
  };

  // 渲染协作者列表
  const renderCollaboratorList = () => {
    if (collaborators.length === 0) {
      return (
        <div style={{ textAlign: 'center', padding: '10px' }}>
          {t('noCollaborators')}
        </div>
      );
    }

    return (
      <List
        itemLayout="horizontal"
        dataSource={collaborators}
        renderItem={item => {
          let avatar;
          if (item.type === 'member') {
            avatar = <Avatar src={item.avatar} />;
          } else if (item.type === 'department') {
            avatar = <Avatar icon={<TeamOutlined />} />;
          } else {
            avatar = <Avatar icon={<UsergroupAddOutlined />} />;
          }

          return (
            <List.Item
              actions={[
                <Button
                  type="text"
                  danger
                  onClick={() => removeCollaborator(item.id, item.type)}
                >
                  {t('remove')}
                </Button>,
              ]}
            >
              <List.Item.Meta avatar={avatar} title={item.name} />
            </List.Item>
          );
        }}
      />
    );
  };

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '10px' }}>
        <span style={{ fontWeight: 'bold' }}>{t('collaborators')}</span>
        <Button type="primary" onClick={showModal}>
          {t('addCollaborator')}
        </Button>
      </div>
      
      <div style={{ border: '1px solid #f0f0f0', borderRadius: '4px', padding: '10px' }}>
        {renderCollaboratorList()}
      </div>

      <Modal
        title={t('addCollaborator')}
        open={isModalVisible}
        onOk={handleOk}
        onCancel={handleCancel}
        width={600}
        maskClosable={false}
      >
        <Tabs
          activeKey={activeTab}
          onChange={setActiveTab}
          items={getTabItems()}
        />
      </Modal>
    </div>
  );
};

export default CollaboratorManagement;