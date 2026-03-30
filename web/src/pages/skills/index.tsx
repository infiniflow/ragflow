import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Segmented } from '@/components/ui/segmented';
import { Spin } from '@/components/ui/spin';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { PageContainer } from '@/layouts/components/page-container';
import {
  FolderOpen,
  LayoutGrid,
  List,
  Plus,
  RefreshCw,
  Search,
} from 'lucide-react';
import React, { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import SkillCard from './components/skill-card';
import SkillDetail from './components/skill-detail';
import UploadModal from './components/upload-modal';
import { useSkills } from './hooks';
import type { Skill } from './types';

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
    getSkillVersionFiles,
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
    async (name: string, version: string, files: File[]) => {
      return await uploadSkill(name, version, files);
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
    <TooltipProvider>
      <PageContainer>
        <div className="min-h-[calc(100vh-200px)]">
          {/* Header */}
          <div className="mb-6">
            <div className="flex justify-between items-center mb-4">
              <h2 className="text-2xl font-semibold">{t('skills.title')}</h2>
              <div className="flex items-center gap-2">
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="outline"
                      size="icon"
                      onClick={fetchSkills}
                      disabled={loading}
                    >
                      <RefreshCw className={loading ? 'animate-spin' : ''} />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t('common.refresh')}</TooltipContent>
                </Tooltip>
                <Button onClick={() => setUploadModalOpen(true)}>
                  <Plus className="mr-2" />
                  {t('skills.uploadSkill')}
                </Button>
              </div>
            </div>

            {/* Search and View Controls */}
            <div className="flex justify-between items-center">
              <Input
                placeholder={t('skills.searchPlaceholder')}
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="w-[300px]"
                prefix={<Search className="size-4 text-text-secondary" />}
              />
              <Segmented
                value={viewMode}
                onChange={(v) => setViewMode(v as 'grid' | 'list')}
                options={[
                  { value: 'grid', label: <LayoutGrid className="size-4" /> },
                  { value: 'list', label: <List className="size-4" /> },
                ]}
              />
            </div>
          </div>

          {/* Skills List */}
          {loading ? (
            <div className="flex justify-center py-16">
              <Spin size="large" />
            </div>
          ) : sortedSkills.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 text-text-secondary">
              <FolderOpen className="size-16 mb-4 opacity-50" />
              {searchQuery ? (
                <p>
                  {t('skills.noSearchResults')}: "{searchQuery}"
                </p>
              ) : (
                <div className="text-center">
                  <p className="mb-2">{t('skills.noSkills')}</p>
                  <button
                    className="text-accent-primary hover:underline"
                    onClick={() => setUploadModalOpen(true)}
                  >
                    {t('skills.uploadSkill')}
                  </button>
                </div>
              )}
            </div>
          ) : (
            <div
              className="grid gap-4"
              style={{
                gridTemplateColumns:
                  viewMode === 'grid'
                    ? 'repeat(auto-fill, minmax(360px, 1fr))'
                    : '1fr',
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
        </div>

        {/* Skill Detail Drawer */}
        <SkillDetail
          skill={selectedSkill}
          open={detailOpen}
          onClose={handleCloseDetail}
          getFileContent={getSkillFileContent}
          getVersionFiles={getSkillVersionFiles}
        />

        {/* Upload Modal */}
        <UploadModal
          open={uploadModalOpen}
          onCancel={() => setUploadModalOpen(false)}
          onUpload={handleUpload}
          loading={loading}
        />
      </PageContainer>
    </TooltipProvider>
  );
};

export default SkillsPage;
