import { CardContainer } from '@/components/card-container';
import { EmptyCardType } from '@/components/empty/constant';
import { EmptyAppCard } from '@/components/empty/empty';
import ListFilterBar from '@/components/list-filter-bar';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Segmented } from '@/components/ui/segmented';
import { Spin } from '@/components/ui/spin';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { Routes } from '@/routes';
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
import { useLocation, useNavigate } from 'react-router';
import SearchConfigModal from './components/search-config-modal';
import SkillCard from './components/skill-card';
import SkillDetail from './components/skill-detail';
import UploadModal from './components/upload-modal';
import { useSkills, useSkillSearchConfig } from './hooks';
import type { Skill } from './types';

// Format relative time
const formatRelative = (timestamp: number): string => {
  let normalized = timestamp;
  if (normalized > 1e17) normalized = normalized / 1e6;
  else if (normalized > 1e14) normalized = normalized / 1e3;
  else if (normalized > 0 && normalized < 1e11) normalized = normalized * 1e3;

  const diff = Date.now() - normalized;
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
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const [hubs, setHubs] = useState<Array<{ id: string; name: string }>>([]);
  const [hubInput, setHubInput] = useState('');
  const [selectedHubId, setSelectedHubId] = useState<string>('');
  const [selectedHubName, setSelectedHubName] = useState<string>('');
  const [hubLoading, setHubLoading] = useState(false);
  const [hubSearchString, setHubSearchString] = useState('');

  const {
    skills,
    filteredSkills,
    loading,
    searchQuery,
    setSearchQuery,
    fetchHubs,
    createHub,
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
  } = useSkillSearchConfig(selectedHubId);

  const [viewMode, setViewMode] = useState<'grid' | 'list'>('grid');
  const [detailOpen, setDetailOpen] = useState(false);
  const [selectedSkill, setSelectedSkill] = useState<Skill | null>(null);
  const [uploadModalOpen, setUploadModalOpen] = useState(false);
  const [configModalOpen, setConfigModalOpen] = useState(false);
  const [createHubModalOpen, setCreateHubModalOpen] = useState(false);
  const [searchResults, setSearchResults] = useState<Skill[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [hasSearched, setHasSearched] = useState(false);

  const clearModalLocks = useCallback(() => {
    setDetailOpen(false);
    setUploadModalOpen(false);
    setConfigModalOpen(false);
    setSelectedSkill(null);
    document.body.style.removeProperty('pointer-events');
    document.body.style.removeProperty('overflow');
  }, []);

  useEffect(() => {
    clearModalLocks();
  }, [pathname, clearModalLocks]);

  useEffect(() => {
    return () => {
      document.body.style.removeProperty('pointer-events');
      document.body.style.removeProperty('overflow');
    };
  }, []);

  const loadHubs = useCallback(async () => {
    setHubLoading(true);
    try {
      const nextHubs = await fetchHubs();
      setHubs(nextHubs);
    } finally {
      setHubLoading(false);
    }
  }, [fetchHubs]);

  useEffect(() => {
    loadHubs();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (!selectedHubId) return;
    fetchConfig(undefined, selectedHubId);
    fetchSkills(selectedHubId);
  }, [selectedHubId, fetchConfig, fetchSkills]);

  const handleViewSkill = useCallback(
    (skill: Skill) => {
      if (!(skill as any)._folderId) {
        const existingSkill = filteredSkills.find((s) => s.id === skill.id);
        if (existingSkill && (existingSkill as any)._folderId) {
          skill = { ...skill, _folderId: (existingSkill as any)._folderId };
        } else {
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
      return await uploadSkill(name, version, files, selectedHubId);
    },
    [uploadSkill, selectedHubId],
  );

  const handleDelete = useCallback(
    async (skillId: string, skillName: string) => {
      await deleteSkill(skillId, skillName, selectedHubId);
    },
    [deleteSkill, selectedHubId],
  );

  const handleCreateHub = useCallback(async () => {
    const nextHubName = hubInput.trim();
    if (!nextHubName) return;
    const newHub = await createHub(nextHubName);
    if (!newHub) return;
    setHubInput('');
    setCreateHubModalOpen(false);
    await loadHubs();
    // Select the newly created hub
    setSelectedHubId(newHub.id);
    setSelectedHubName(newHub.name);
  }, [hubInput, createHub, loadHubs]);

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
          const localSkillMap = new Map(skills.map((s) => [s.id, s]));
          const localSkillNameMap = new Map(
            skills.map((s) => [s.name.toLowerCase(), s]),
          );
          const mergedResults = results.skills.map((skill) => {
            const localSkill =
              localSkillMap.get(skill.id) ||
              localSkillNameMap.get(skill.name.toLowerCase());
            if (!localSkill) return skill;
            return {
              ...skill,
              created_at: localSkill.created_at,
              updated_at: localSkill.updated_at,
              _folderId:
                (skill as any)._folderId || (localSkill as any)._folderId,
            };
          });
          setSearchResults(mergedResults);
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
    [searchSkills, setSearchQuery, skills],
  );

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

  const handleSearchKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'Enter') {
        handleSearch(searchQuery);
      }
    },
    [handleSearch, searchQuery],
  );

  const handleSearchClick = useCallback(() => {
    handleSearch(searchQuery);
  }, [handleSearch, searchQuery]);

  const handleHubSearchChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setHubSearchString(e.target.value);
    },
    [],
  );

  const filteredHubs = useMemo(() => {
    if (!hubSearchString.trim()) return hubs;
    return hubs.filter((hub) =>
      hub.name.toLowerCase().includes(hubSearchString.toLowerCase()),
    );
  }, [hubs, hubSearchString]);

  const displayedSkills = useMemo(() => {
    const skills = hasSearched ? searchResults : filteredSkills;
    return [...skills].sort((a, b) => b.updated_at - a.updated_at);
  }, [hasSearched, searchResults, filteredSkills]);

  const isLoading = loading || isSearching || configLoading;

  // Hub list breadcrumb: root / skills
  const hubListBreadcrumb = (
    <div className="flex items-center gap-2">
      <span
        className="text-text-secondary cursor-pointer hover:text-text-primary"
        onClick={() => navigate(Routes.Files)}
      >
        root
      </span>
      <span className="text-text-secondary">/</span>
      <span>{t('skills.title')}</span>
    </div>
  );

  // Skills list breadcrumb: root / skills / {hubName}
  const skillsListBreadcrumb = (
    <div className="flex items-center gap-2">
      <span
        className="text-text-secondary cursor-pointer hover:text-text-primary"
        onClick={() => navigate(Routes.Files)}
      >
        root
      </span>
      <span className="text-text-secondary">/</span>
      <span
        className="text-text-secondary cursor-pointer hover:text-text-primary"
        onClick={() => {
          setSelectedHubId('');
          setSelectedHubName('');
        }}
      >
        {t('skills.title')}
      </span>
      <span className="text-text-secondary">/</span>
      <span>{selectedHubName}</span>
    </div>
  );

  // Hub list page (no hub selected)
  if (!selectedHubId) {
    return (
      <>
        <article
          className="size-full flex flex-col"
          data-testid="skills-hub-list"
        >
          <header className="px-5 pt-8 mb-4">
            <ListFilterBar
              leftPanel={hubListBreadcrumb}
              searchString={hubSearchString}
              onSearchChange={handleHubSearchChange}
              showFilter={false}
              icon="skills"
            >
              <Button onClick={() => setCreateHubModalOpen(true)}>
                <Plus className="size-[1em]" />
                {t('skills.createHub') || 'Create Skills Hub'}
              </Button>
            </ListFilterBar>
          </header>

          <div className="flex-1 px-5 flex flex-col overflow-hidden">
            {hubLoading ? (
              <div className="flex-1 flex items-center justify-center">
                <Spin size="large" />
              </div>
            ) : filteredHubs.length ? (
              <CardContainer className="flex-1 overflow-auto">
                {filteredHubs.map((hub) => (
                  <div
                    key={hub.id}
                    className="group flex flex-col rounded-xl border border-border p-4 hover:border-accent-primary hover:shadow-md transition-all cursor-pointer bg-bg-card"
                    onClick={() => {
                      setSelectedHubId(hub.id);
                      setSelectedHubName(hub.name);
                    }}
                  >
                    <div className="flex items-start justify-between mb-2">
                      <div className="flex-1 min-w-0">
                        <h3 className="font-semibold text-lg truncate">
                          {hub.name}
                        </h3>
                      </div>
                    </div>
                    <div className="mt-auto pt-2">
                      <span className="text-accent-primary text-sm">
                        {t('skills.enterHub') || 'Enter'} →
                      </span>
                    </div>
                  </div>
                ))}
              </CardContainer>
            ) : (
              <div className="flex-1 flex items-center justify-center">
                {hubSearchString ? (
                  <EmptyAppCard
                    showIcon
                    size="large"
                    className="w-[480px] p-14"
                    isSearch
                    type={EmptyCardType.Skills}
                  />
                ) : (
                  <EmptyAppCard
                    showIcon
                    size="large"
                    className="w-[480px] p-14"
                    type={EmptyCardType.Skills}
                    onClick={() => setCreateHubModalOpen(true)}
                  />
                )}
              </div>
            )}
          </div>
        </article>

        {/* Create Hub Modal */}
        <Dialog open={createHubModalOpen} onOpenChange={setCreateHubModalOpen}>
          <DialogContent className="sm:max-w-[425px]">
            <DialogHeader>
              <DialogTitle>
                {t('skills.createHubTitle') || 'Create New Skills Hub'}
              </DialogTitle>
              <DialogDescription>
                {t('skills.createHubDescription') ||
                  'Create a new hub to organize and manage your skills.'}
              </DialogDescription>
            </DialogHeader>
            <div className="py-4">
              <label className="text-sm font-medium mb-2 block">
                {t('skills.hubName') || 'Hub Name'}
              </label>
              <Input
                placeholder={t('skills.hubNamePlaceholder') || 'e.g., my-hub'}
                value={hubInput}
                onChange={(e) => setHubInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && hubInput.trim()) {
                    handleCreateHub();
                  }
                }}
              />
            </div>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => {
                  setCreateHubModalOpen(false);
                  setHubInput('');
                }}
              >
                {t('common.cancel')}
              </Button>
              <Button onClick={handleCreateHub} disabled={!hubInput.trim()}>
                {t('common.create')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </>
    );
  }

  // Inside a hub (skills list page)
  return (
    <article className="size-full flex flex-col" data-testid="skills-list">
      <header className="px-5 pt-8 mb-4">
        <ListFilterBar
          leftPanel={skillsListBreadcrumb}
          showFilter={false}
          icon="skills"
        >
          <div className="flex items-center gap-2">
            <TooltipProvider>
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
                    onClick={() => fetchSkills(selectedHubId)}
                    disabled={loading}
                  >
                    <RefreshCw className={loading ? 'animate-spin' : ''} />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{t('common.refresh')}</TooltipContent>
              </Tooltip>
            </TooltipProvider>
            <Button onClick={() => setUploadModalOpen(true)}>
              <Plus className="mr-2" />
              {t('skills.addSkill') || 'Add Skill'}
            </Button>
          </div>
        </ListFilterBar>
      </header>

      <div className="flex-1 px-5 flex flex-col overflow-hidden">
        {/* Search and View Controls */}
        <div className="flex justify-between items-center mb-4">
          <div className="relative">
            <Input
              placeholder={t('skills.searchPlaceholder') || 'Search skills...'}
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

        {/* Skills List */}
        {isLoading ? (
          <div className="flex-1 flex items-center justify-center">
            <Spin size="large" />
          </div>
        ) : displayedSkills.length === 0 ? (
          <div className="flex-1 flex flex-col items-center justify-center text-text-secondary">
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
          <CardContainer className="flex-1 overflow-auto">
            {displayedSkills.map((skill) => (
              <SkillCard
                key={skill.id}
                skill={skill}
                onView={handleViewSkill}
                onDelete={handleDelete}
                formatRelative={formatRelative}
              />
            ))}
          </CardContainer>
        )}
      </div>

      {/* Skill Detail Drawer */}
      {detailOpen && selectedSkill && (
        <SkillDetail
          skill={selectedSkill}
          open={detailOpen}
          onClose={handleCloseDetail}
          getFileContent={getSkillFileContent}
          getVersionFiles={getSkillVersionFiles}
        />
      )}

      {/* Upload Modal */}
      {uploadModalOpen && (
        <UploadModal
          open={uploadModalOpen}
          onCancel={() => setUploadModalOpen(false)}
          onUpload={handleUpload}
        />
      )}

      {/* Search Config Modal */}
      {configModalOpen && (
        <SearchConfigModal
          open={configModalOpen}
          onOpenChange={setConfigModalOpen}
          config={config || undefined}
          onSave={saveConfig}
          onReindex={reindex}
          loading={configLoading}
        />
      )}
    </article>
  );
};

export default SkillsPage;
