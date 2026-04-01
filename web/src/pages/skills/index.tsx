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
  Settings,
} from 'lucide-react';
import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import SearchConfigModal from './components/search-config-modal';
import SkillCard from './components/skill-card';
import SkillDetail from './components/skill-detail';
import UploadModal from './components/upload-modal';
import { useSkills, useSkillSearchConfig } from './hooks';
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

  const {
    config,
    configLoading,
    saveConfig,
    fetchConfig,
    reindex,
    searchSkills,
  } = useSkillSearchConfig();

  const [viewMode, setViewMode] = useState<'grid' | 'list'>('grid');
  const [detailOpen, setDetailOpen] = useState(false);
  const [selectedSkill, setSelectedSkill] = useState<Skill | null>(null);
  const [uploadModalOpen, setUploadModalOpen] = useState(false);
  const [configModalOpen, setConfigModalOpen] = useState(false);
  const [searchResults, setSearchResults] = useState<Skill[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [hasSearched, setHasSearched] = useState(false);

  // Fetch config on mount - only once
  useEffect(() => {
    fetchConfig();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Handle view skill - search results need _folderId from existing skills
  const handleViewSkill = useCallback(
    (skill: Skill) => {
      // If skill has no _folderId (from search results), try to find it in skills list
      if (!(skill as any)._folderId) {
        // Use a closure to capture filteredSkills at call time
        const existingSkill = filteredSkills.find((s) => s.id === skill.id);
        if (existingSkill && (existingSkill as any)._folderId) {
          // Merge _folderId from existing skill
          skill = { ...skill, _folderId: (existingSkill as any)._folderId };
        } else {
          // Search result doesn't have _folderId and skill not in local list
          // This happens when index is outdated (folder_id not stored in ES)
          console.warn(
            `[Skill Search] Skill "${skill.name}" has no folder_id. ` +
              'Please reindex skills to fix this issue.',
          );
        }
      }
      setSelectedSkill(skill);
      setDetailOpen(true);
    },
    [filteredSkills],
  );

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
    async (skillId: string, skillName: string) => {
      await deleteSkill(skillId, skillName);
    },
    [deleteSkill],
  );

  // Handle search - all searches go through skillsearch API
  const handleSearch = useCallback(
    async (query: string) => {
      setSearchQuery(query);

      if (!query.trim()) {
        setSearchResults([]);
        setHasSearched(false);
        return;
      }

      setIsSearching(true);
      setHasSearched(true);
      try {
        const results = await searchSkills(query, 1, 20);
        if (results?.skills) {
          setSearchResults(results.skills);
        } else {
          setSearchResults([]);
        }
      } catch (error) {
        console.error('Search error:', error);
        setSearchResults([]);
      } finally {
        setIsSearching(false);
      }
    },
    [searchSkills, setSearchQuery],
  );

  // Handle search input change (for controlled input)
  const handleSearchInputChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const value = e.target.value;
      setSearchQuery(value);
      if (!value.trim()) {
        setSearchResults([]);
        setHasSearched(false);
      }
    },
    [setSearchQuery],
  );

  // Handle search on Enter key
  const handleSearchKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'Enter') {
        handleSearch(searchQuery);
      }
    },
    [handleSearch, searchQuery],
  );

  // Handle search button click
  const handleSearchClick = useCallback(() => {
    handleSearch(searchQuery);
  }, [handleSearch, searchQuery]);

  // Determine which skills to display
  const displayedSkills = useMemo(() => {
    // If search is active, show search results; otherwise show all skills
    const skills = hasSearched ? searchResults : filteredSkills;
    return [...skills].sort((a, b) => b.updated_at - a.updated_at);
  }, [hasSearched, searchResults, filteredSkills]);

  // Show loading state
  const isLoading = loading || isSearching || configLoading;

  return (
    <PageContainer>
      <div className="min-h-[calc(100vh-200px)]">
        {/* Header */}
        <div className="mb-6">
          <div className="flex justify-between items-center mb-4">
            <h2 className="text-2xl font-semibold">{t('skills.title')}</h2>
            <TooltipProvider>
              <div className="flex items-center gap-2">
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="outline"
                      size="icon"
                      onClick={() => setConfigModalOpen(true)}
                      disabled={loading}
                    >
                      <Settings className="size-4" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t('skills.configureSearch')}</TooltipContent>
                </Tooltip>
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
                  {t('skills.addSkill') || 'Add Skill'}
                </Button>
              </div>
            </TooltipProvider>
          </div>

          {/* Search and View Controls */}
          <div className="flex justify-between items-center">
            <div className="relative">
              <Input
                placeholder={
                  t('skills.searchPlaceholder') || 'Search skills...'
                }
                value={searchQuery}
                onChange={handleSearchInputChange}
                onKeyDown={handleSearchKeyDown}
                className="w-[300px] pr-10"
              />
              <button
                onClick={handleSearchClick}
                disabled={isSearching}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-text-secondary hover:text-text-primary disabled:opacity-50 disabled:cursor-not-allowed"
              >
                <Search className="size-4" />
              </button>
            </div>
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
        {isLoading ? (
          <div className="flex justify-center py-16">
            <Spin size="large" />
          </div>
        ) : displayedSkills.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 text-text-secondary">
            <FolderOpen className="size-16 mb-4 opacity-50" />
            {hasSearched ? (
              <p>
                {t('skills.noSearchResults') || 'No search results'}: "
                {searchQuery}"
              </p>
            ) : searchQuery ? (
              <p>
                {t('skills.noSearchResults') || 'No search results'}: "
                {searchQuery}"
              </p>
            ) : (
              <div className="text-center">
                <p className="mb-2">{t('skills.noSkills')}</p>
                <button
                  className="text-accent-primary hover:underline"
                  onClick={() => setUploadModalOpen(true)}
                >
                  {t('skills.addSkill') || 'Add Skill'}
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
            {displayedSkills.map((skill) => (
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

      {/* Search Config Modal */}
      <SearchConfigModal
        open={configModalOpen}
        onOpenChange={setConfigModalOpen}
        config={config || undefined}
        onSave={saveConfig}
        onReindex={reindex}
        loading={configLoading}
      />
    </PageContainer>
  );
};

export default SkillsPage;
