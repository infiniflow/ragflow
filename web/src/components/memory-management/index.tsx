import { useTranslate } from '@/hooks/common-hooks';
import { useChatStore } from '@/stores/chat-store';
import {
  BrainOutlined,
  DeleteOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import {
  Button,
  Card,
  Empty,
  Input,
  List,
  Modal,
  Popconfirm,
  Space,
  Spin,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd';
import { useCallback, useEffect, useState } from 'react';

const { Search } = Input;
const { Text, Paragraph } = Typography;

interface Memory {
  id: string;
  content: string;
  score?: number;
  metadata?: Record<string, any>;
  created_at?: string;
}

interface MemoryStats {
  total_memories: number;
  enabled: boolean;
  last_updated?: string;
}

interface MemoryManagementProps {
  visible: boolean;
  onClose: () => void;
  chatId: string;
}

const MemoryManagement: React.FC<MemoryManagementProps> = ({
  visible,
  onClose,
  chatId,
}) => {
  const { t } = useTranslate('chat');
  const [memories, setMemories] = useState<Memory[]>([]);
  const [filteredMemories, setFilteredMemories] = useState<Memory[]>([]);
  const [stats, setStats] = useState<MemoryStats>({
    total_memories: 0,
    enabled: false,
  });
  const [loading, setLoading] = useState(false);
  const [searchLoading, setSearchLoading] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const {
    fetchMemories,
    deleteMemory,
    clearMemories,
    searchMemories,
    getMemoryStats,
  } = useChatStore();

  const loadMemories = useCallback(async () => {
    if (!chatId) return;

    setLoading(true);
    try {
      const [memoriesData, statsData] = await Promise.all([
        fetchMemories(chatId),
        getMemoryStats(chatId),
      ]);
      setMemories(memoriesData);
      setFilteredMemories(memoriesData);
      setStats(statsData);
    } catch (error) {
      message.error(t('failedToLoadMemories'));
    } finally {
      setLoading(false);
    }
  }, [chatId, fetchMemories, getMemoryStats, t]);

  const handleSearch = useCallback(
    async (query: string) => {
      setSearchQuery(query);

      if (!query.trim()) {
        setFilteredMemories(memories);
        return;
      }

      setSearchLoading(true);
      try {
        const searchResults = await searchMemories(chatId, query);
        setFilteredMemories(searchResults);
      } catch (error) {
        message.error(t('failedToSearchMemories'));
        setFilteredMemories(memories);
      } finally {
        setSearchLoading(false);
      }
    },
    [chatId, memories, searchMemories, t],
  );

  const handleDeleteMemory = useCallback(
    async (memoryId: string) => {
      try {
        await deleteMemory(chatId, memoryId);
        message.success(t('memoryDeleted'));
        loadMemories();
      } catch (error) {
        message.error(t('failedToDeleteMemory'));
      }
    },
    [chatId, deleteMemory, loadMemories, t],
  );

  const handleClearAllMemories = useCallback(async () => {
    try {
      await clearMemories(chatId);
      message.success(t('allMemoriesCleared'));
      loadMemories();
    } catch (error) {
      message.error(t('failedToClearMemories'));
    }
  }, [chatId, clearMemories, loadMemories, t]);

  const formatDate = useCallback(
    (dateString?: string) => {
      if (!dateString) return t('unknown');
      return new Date(dateString).toLocaleString();
    },
    [t],
  );

  useEffect(() => {
    if (visible) {
      loadMemories();
    }
  }, [visible, loadMemories]);

  const renderMemoryItem = (memory: Memory) => (
    <List.Item
      key={memory.id}
      actions={[
        <Tooltip title={t('deleteMemory')}>
          <Popconfirm
            title={t('confirmDeleteMemory')}
            onConfirm={() => handleDeleteMemory(memory.id)}
            okText={t('delete', { keyPrefix: 'common' })}
            cancelText={t('cancel', { keyPrefix: 'common' })}
          >
            <Button type="text" icon={<DeleteOutlined />} danger size="small" />
          </Popconfirm>
        </Tooltip>,
      ]}
    >
      <List.Item.Meta
        avatar={
          <BrainOutlined style={{ fontSize: '16px', color: '#1890ff' }} />
        }
        title={
          <Space>
            <Text strong>{t('memory')}</Text>
            {memory.score && (
              <Tag color="blue">
                {t('relevance')}: {(memory.score * 100).toFixed(1)}%
              </Tag>
            )}
          </Space>
        }
        description={
          <div>
            <Paragraph
              ellipsis={{ rows: 3, expandable: true, symbol: t('showMore') }}
              style={{ marginBottom: 8 }}
            >
              {memory.content}
            </Paragraph>
            <Text type="secondary" style={{ fontSize: '12px' }}>
              {t('created')}: {formatDate(memory.created_at)}
            </Text>
          </div>
        }
      />
    </List.Item>
  );

  return (
    <Modal
      title={
        <Space>
          <BrainOutlined />
          {t('memoryManagement')}
          {stats.enabled && (
            <Tag color="green">
              {t('memoriesCount', { count: stats.total_memories })}
            </Tag>
          )}
        </Space>
      }
      open={visible}
      onCancel={onClose}
      width={800}
      footer={[
        <Popconfirm
          key="clear"
          title={t('confirmClearAllMemories')}
          onConfirm={handleClearAllMemories}
          okText={t('clear', { keyPrefix: 'common' })}
          cancelText={t('cancel', { keyPrefix: 'common' })}
          disabled={!stats.enabled || stats.total_memories === 0}
        >
          <Button
            danger
            disabled={!stats.enabled || stats.total_memories === 0}
          >
            {t('clearAllMemories')}
          </Button>
        </Popconfirm>,
        <Button key="close" onClick={onClose}>
          {t('close', { keyPrefix: 'common' })}
        </Button>,
      ]}
    >
      <div style={{ marginBottom: 16 }}>
        {!stats.enabled ? (
          <Card>
            <Empty
              image={
                <BrainOutlined style={{ fontSize: '48px', color: '#d9d9d9' }} />
              }
              description={t('memoryNotEnabled')}
            />
          </Card>
        ) : (
          <>
            <Search
              placeholder={t('searchMemoriesPlaceholder')}
              allowClear
              enterButton={<SearchOutlined />}
              size="large"
              onSearch={handleSearch}
              loading={searchLoading}
              style={{ marginBottom: 16 }}
            />

            <Spin spinning={loading}>
              {filteredMemories.length === 0 ? (
                <Empty
                  image={
                    <BrainOutlined
                      style={{ fontSize: '48px', color: '#d9d9d9' }}
                    />
                  }
                  description={
                    searchQuery
                      ? t('noMemoriesFoundForSearch')
                      : t('noMemoriesYet')
                  }
                />
              ) : (
                <List
                  itemLayout="vertical"
                  dataSource={filteredMemories}
                  renderItem={renderMemoryItem}
                  pagination={{
                    pageSize: 5,
                    showSizeChanger: false,
                    showQuickJumper: true,
                  }}
                />
              )}
            </Spin>

            {stats.last_updated && (
              <Text
                type="secondary"
                style={{
                  fontSize: '12px',
                  display: 'block',
                  textAlign: 'center',
                  marginTop: 16,
                }}
              >
                {t('lastUpdated')}: {formatDate(stats.last_updated)}
              </Text>
            )}
          </>
        )}
      </div>
    </Modal>
  );
};

export default MemoryManagement;
