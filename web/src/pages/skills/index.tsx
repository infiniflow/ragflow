import { PageContainer } from '@/layouts/components/page-container';
import {
  AppstoreOutlined,
  PlusOutlined,
  ReloadOutlined,
  SearchOutlined,
  UnorderedListOutlined,
} from '@ant-design/icons';
import {
  Button,
  Empty,
  Input,
  Layout,
  Segmented,
  Space,
  Spin,
  Tooltip,
  Typography,
} from 'antd';
import React, { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import SkillCard from './components/skill-card';
import SkillDetail from './components/skill-detail';
import UploadModal from './components/upload-modal';
import { useSkills } from './hooks';
import type { Skill } from './types';

const { Title } = Typography;
const { Content } = Layout;

// Format relative time
const formatRelative = (timestamp: number): string => {
  const diff = Date.now() - timestamp;
  if (diff < 0) return 'just now';

  const minutes = Math.floor(diff / 60000);
  if (minutes < 1) return 'just now';
  if (minutes < 60) return `${minutes}m ago`;

  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;

  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;

  const months = Math.floor(days / 30);
  if (months < 12) return `${months}mo ago`;

  const years = Math.floor(months / 12);
  return `${years}y ago`;
};

const SkillsPage: React.FC = () => {
  const { t } = useTranslation();
  const {
    filteredSkills,
    loading,
    searchQuery,
    setSearchQuery,
    fetchSkills,
    uploadSkill,
    deleteSkill,
    getSkillFileContent,
  } = useSkills();

  const [viewMode, setViewMode] = useState<'grid' | 'list'>('grid');
  const [detailOpen, setDetailOpen] = useState(false);
  const [selectedSkill, setSelectedSkill] = useState<Skill | null>(null);
  const [uploadModalOpen, setUploadModalOpen] = useState(false);

  const handleViewSkill = useCallback((skill: Skill) => {
    setSelectedSkill(skill);
    setDetailOpen(true);
  }, []);

  const handleCloseDetail = useCallback(() => {
    setDetailOpen(false);
    setSelectedSkill(null);
  }, []);

  const handleUpload = useCallback(
    async (name: string, files: File[]) => {
      return await uploadSkill(name, files);
    },
    [uploadSkill],
  );

  const handleDelete = useCallback(
    async (skillId: string) => {
      await deleteSkill(skillId);
    },
    [deleteSkill],
  );

  const sortedSkills = useMemo(() => {
    return [...filteredSkills].sort((a, b) => b.updated_at - a.updated_at);
  }, [filteredSkills]);

  return (
    <PageContainer>
      <Layout
        style={{ minHeight: 'calc(100vh - 200px)', background: 'transparent' }}
      >
        <Content>
          {/* Header */}
          <div style={{ marginBottom: 24 }}>
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                marginBottom: 16,
              }}
            >
              <Title level={2} style={{ margin: 0 }}>
                {t('skills.title')}
              </Title>
              <Space>
                <Tooltip title={t('common.refresh')}>
                  <Button
                    icon={<ReloadOutlined />}
                    onClick={fetchSkills}
                    loading={loading}
                  />
                </Tooltip>
                <Button
                  type="primary"
                  icon={<PlusOutlined />}
                  onClick={() => setUploadModalOpen(true)}
                >
                  {t('skills.uploadSkill')}
                </Button>
              </Space>
            </div>

            {/* Search and View Controls */}
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
              }}
            >
              <Input
                placeholder={t('skills.searchPlaceholder')}
                prefix={<SearchOutlined />}
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                style={{ width: 300 }}
                allowClear
              />
              <Segmented
                value={viewMode}
                onChange={(v) => setViewMode(v as 'grid' | 'list')}
                options={[
                  { value: 'grid', icon: <AppstoreOutlined /> },
                  { value: 'list', icon: <UnorderedListOutlined /> },
                ]}
              />
            </div>
          </div>

          {/* Skills List */}
          {loading ? (
            <div
              style={{ display: 'flex', justifyContent: 'center', padding: 60 }}
            >
              <Spin size="large" />
            </div>
          ) : sortedSkills.length === 0 ? (
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={
                searchQuery ? (
                  <span>
                    {t('skills.noSearchResults')}: "{searchQuery}"
                  </span>
                ) : (
                  <span>
                    {t('skills.noSkills')}.{' '}
                    <a onClick={() => setUploadModalOpen(true)}>
                      {t('skills.uploadSkill')}
                    </a>
                  </span>
                )
              }
              style={{ marginTop: 60 }}
            />
          ) : (
            <div
              style={{
                display: 'grid',
                gridTemplateColumns:
                  viewMode === 'grid'
                    ? 'repeat(auto-fill, minmax(360px, 1fr))'
                    : '1fr',
                gap: 16,
              }}
            >
              {sortedSkills.map((skill) => (
                <SkillCard
                  key={skill.id}
                  skill={skill}
                  onView={handleViewSkill}
                  onDelete={handleDelete}
                  formatRelative={formatRelative}
                />
              ))}
            </div>
          )}
        </Content>
      </Layout>

      {/* Skill Detail Drawer */}
      <SkillDetail
        skill={selectedSkill}
        open={detailOpen}
        onClose={handleCloseDetail}
        getFileContent={getSkillFileContent}
      />

      {/* Upload Modal */}
      <UploadModal
        open={uploadModalOpen}
        onCancel={() => setUploadModalOpen(false)}
        onUpload={handleUpload}
        loading={loading}
      />
    </PageContainer>
  );
};

export default SkillsPage;
