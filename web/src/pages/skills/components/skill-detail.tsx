import {
  ArrowLeftOutlined,
  FileMarkdownOutlined,
  FileOutlined,
  FolderOutlined,
} from '@ant-design/icons';
import {
  Button,
  Drawer,
  Empty,
  message,
  Space,
  Spin,
  Tag,
  Tree,
  Typography,
} from 'antd';
import React, { memo, useCallback, useEffect, useState } from 'react';
import { isMarkdownFile } from '../hooks';
import type { Skill, SkillFileEntry } from '../types';
import CodeViewer from './code-viewer';
import MarkdownViewer from './markdown-viewer';

const { Title, Text } = Typography;

interface SkillDetailProps {
  skill: Skill | null;
  open: boolean;
  onClose: () => void;
  getFileContent: (skillId: string, fileName: string) => Promise<string | null>;
}

interface TreeNode {
  title: string;
  key: string;
  isDir: boolean;
  children?: TreeNode[];
  icon?: React.ReactNode;
}

const getFileIcon = (filename: string, isDir: boolean) => {
  if (isDir) return <FolderOutlined />;
  if (isMarkdownFile(filename))
    return <FileMarkdownOutlined style={{ color: '#1890ff' }} />;
  return <FileOutlined />;
};

// Build tree from flat file list
const buildFileTree = (files: SkillFileEntry[]): TreeNode[] => {
  const root: TreeNode[] = [];
  const map: Record<string, TreeNode> = {};

  // Sort files: directories first, then alphabetically
  const sortedFiles = [...files].sort((a, b) => {
    if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;
    return a.name.localeCompare(b.name);
  });

  sortedFiles.forEach((file) => {
    const parts = file.path.split('/');
    const name = parts[parts.length - 1];

    const node: TreeNode = {
      title: name,
      key: file.path,
      isDir: file.is_dir,
      icon: getFileIcon(name, file.is_dir),
    };

    if (file.is_dir) {
      node.children = [];
    }

    map[file.path] = node;

    if (parts.length === 1) {
      root.push(node);
    } else {
      const parentPath = parts.slice(0, -1).join('/');
      const parent = map[parentPath];
      if (parent && parent.children) {
        parent.children.push(node);
      }
    }
  });

  return root;
};

const SkillDetail: React.FC<SkillDetailProps> = ({
  skill,
  open,
  onClose,
  getFileContent,
}) => {
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const [fileContent, setFileContent] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [expandedKeys, setExpandedKeys] = useState<string[]>([]);

  const treeData = skill ? buildFileTree(skill.files) : [];

  // Auto-expand first level directories
  useEffect(() => {
    if (skill && treeData.length > 0) {
      const firstLevelDirs = treeData
        .filter((node) => node.isDir)
        .map((node) => node.key);
      setExpandedKeys(firstLevelDirs);
    }
  }, [skill?.id]);

  const handleSelect = useCallback(
    async (selectedKeys: React.Key[]) => {
      if (!skill || selectedKeys.length === 0) return;

      const key = selectedKeys[0] as string;
      const file = skill.files.find((f) => f.path === key);

      if (!file || file.is_dir) return;

      setSelectedFile(key);
      setLoading(true);

      try {
        const content = await getFileContent(skill.id, file.path);
        setFileContent(content || '');
      } catch (error) {
        message.error('Failed to load file content');
      } finally {
        setLoading(false);
      }
    },
    [skill, getFileContent],
  );

  // Auto-select README on open
  useEffect(() => {
    if (skill && open) {
      const readmeFile = skill.files.find(
        (f) =>
          f.name.toLowerCase() === 'readme.md' ||
          f.name.toLowerCase() === 'index.md',
      );
      if (readmeFile && !readmeFile.is_dir) {
        handleSelect([readmeFile.path]);
      }
    }
  }, [skill?.id, open, handleSelect]);

  const renderFileContent = () => {
    if (!selectedFile) {
      return (
        <Empty description="Select a file to view" style={{ marginTop: 100 }} />
      );
    }

    if (loading) {
      return (
        <div style={{ display: 'flex', justifyContent: 'center', padding: 40 }}>
          <Spin size="large" />
        </div>
      );
    }

    const filename = selectedFile.split('/').pop() || '';

    if (isMarkdownFile(filename)) {
      return <MarkdownViewer content={fileContent} />;
    }

    return <CodeViewer content={fileContent} filename={filename} />;
  };

  return (
    <Drawer
      title={null}
      open={open}
      onClose={onClose}
      width="90%"
      styles={{ body: { padding: 0 } }}
      closable={false}
    >
      {skill && (
        <div style={{ display: 'flex', height: '100%' }}>
          {/* Sidebar - File Tree */}
          <div
            style={{
              width: 280,
              borderRight: '1px solid #f0f0f0',
              display: 'flex',
              flexDirection: 'column',
            }}
          >
            <div style={{ padding: 16, borderBottom: '1px solid #f0f0f0' }}>
              <Button
                type="text"
                icon={<ArrowLeftOutlined />}
                onClick={onClose}
                style={{ marginBottom: 8 }}
              >
                Back to Skills
              </Button>
              <Title level={4} style={{ margin: 0, marginTop: 8 }} ellipsis>
                {skill.name}
              </Title>
              {skill.metadata?.description && (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {skill.metadata.description}
                </Text>
              )}
              <Space wrap size="small" style={{ marginTop: 8 }}>
                {skill.metadata?.tags?.map((tag) => (
                  <Tag key={tag}>{tag}</Tag>
                ))}
              </Space>
            </div>

            <div style={{ flex: 1, overflow: 'auto', padding: 8 }}>
              <Tree
                treeData={treeData}
                onSelect={handleSelect}
                selectedKeys={selectedFile ? [selectedFile] : []}
                expandedKeys={expandedKeys}
                onExpand={(keys) => setExpandedKeys(keys as string[])}
                showIcon
                blockNode
              />
            </div>
          </div>

          {/* Main Content */}
          <div style={{ flex: 1, overflow: 'auto', padding: 24 }}>
            {renderFileContent()}
          </div>
        </div>
      )}
    </Drawer>
  );
};

export default memo(SkillDetail);
