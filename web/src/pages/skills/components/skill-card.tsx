import {
  DeleteOutlined,
  EyeOutlined,
  FileTextOutlined,
  FolderOutlined,
  GithubOutlined,
} from '@ant-design/icons';
import {
  Button,
  Card,
  Popconfirm,
  Space,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import React, { memo } from 'react';
import type { Skill } from '../types';

const { Text, Paragraph } = Typography;

interface SkillCardProps {
  skill: Skill;
  onView: (skill: Skill) => void;
  onDelete: (skillId: string) => void;
  formatRelative: (timestamp: number) => string;
}

const SkillCard: React.FC<SkillCardProps> = ({
  skill,
  onView,
  onDelete,
  formatRelative,
}) => {
  const getIcon = () => {
    switch (skill.source_type) {
      case 'git':
        return <GithubOutlined style={{ fontSize: 24, color: '#1890ff' }} />;
      case 'local':
        return <FolderOutlined style={{ fontSize: 24, color: '#52c41a' }} />;
      default:
        return <FileTextOutlined style={{ fontSize: 24, color: '#722ed1' }} />;
    }
  };

  const fileCount = skill.files.filter((f) => !f.is_dir).length;
  const dirCount = skill.files.filter((f) => f.is_dir).length;

  return (
    <Card
      className="skill-card"
      hoverable
      onClick={() => onView(skill)}
      styles={{ body: { padding: 16 } }}
    >
      <div style={{ display: 'flex', gap: 16 }}>
        <div style={{ flexShrink: 0, marginTop: 4 }}>{getIcon()}</div>

        <div style={{ flex: 1, minWidth: 0 }}>
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'flex-start',
            }}
          >
            <Typography.Title
              level={5}
              style={{ margin: 0, marginBottom: 8 }}
              ellipsis
            >
              {skill.name}
            </Typography.Title>

            <Space size="small" onClick={(e) => e.stopPropagation()}>
              <Tooltip title="View">
                <Button
                  type="text"
                  size="small"
                  icon={<EyeOutlined />}
                  onClick={(e) => {
                    e.stopPropagation();
                    onView(skill);
                  }}
                />
              </Tooltip>
              <Popconfirm
                title="Delete Skill"
                description="Are you sure you want to delete this skill?"
                onConfirm={(e) => {
                  e?.stopPropagation();
                  onDelete(skill.id);
                }}
                okText="Delete"
                cancelText="Cancel"
                okButtonProps={{ danger: true }}
              >
                <Tooltip title="Delete">
                  <Button
                    type="text"
                    size="small"
                    danger
                    icon={<DeleteOutlined />}
                    onClick={(e) => e.stopPropagation()}
                  />
                </Tooltip>
              </Popconfirm>
            </Space>
          </div>

          {skill.description && (
            <Paragraph
              type="secondary"
              style={{ marginBottom: 12, fontSize: 13 }}
              ellipsis={{ rows: 2, expandable: false }}
            >
              {skill.description}
            </Paragraph>
          )}

          <Space wrap size="small" style={{ marginBottom: 8 }}>
            {skill.metadata?.tags?.slice(0, 4).map((tag) => (
              <Tag key={tag} color="blue">
                {tag}
              </Tag>
            ))}
            {skill.metadata?.tags && skill.metadata.tags.length > 4 && (
              <Tag>+{skill.metadata.tags.length - 4}</Tag>
            )}
          </Space>

          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              marginTop: 8,
            }}
          >
            <Space size="small">
              <Text type="secondary" style={{ fontSize: 12 }}>
                {fileCount} files{dirCount > 0 ? `, ${dirCount} folders` : ''}
              </Text>
            </Space>

            <Text type="secondary" style={{ fontSize: 12 }}>
              {formatRelative(skill.updated_at)}
            </Text>
          </div>
        </div>
      </div>
    </Card>
  );
};

export default memo(SkillCard);
