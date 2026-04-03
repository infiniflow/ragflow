import { BulkOperateBar } from '@/components/bulk-operate-bar';
import { CardContainer } from '@/components/card-container';
import { EmptyCardType } from '@/components/empty/constant';
import { EmptyAppCard } from '@/components/empty/empty';
import ListFilterBar from '@/components/list-filter-bar';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
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
import fileManagerService from '@/services/file-manager-service';
import { formatFileSize } from '@/utils/common-util';
import { formatDate } from '@/utils/date';
import {
  FolderOpen,
  LayoutGrid,
  List,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Settings,
  Trash2,
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
    deleteHub,
    updateHub,
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
  const [hubViewMode, setHubViewMode] = useState<'grid' | 'list'>('grid');
  const [detailOpen, setDetailOpen] = useState(false);
  const [selectedSkill, setSelectedSkill] = useState<Skill | null>(null);
  const [uploadModalOpen, setUploadModalOpen] = useState(false);
  const [configModalOpen, setConfigModalOpen] = useState(false);
  const [createHubModalOpen, setCreateHubModalOpen] = useState(false);
  const [deleteHubModalOpen, setDeleteHubModalOpen] = useState(false);
  const [hubToDelete, setHubToDelete] = useState<{
    id: string;
    name: string;
  } | null>(null);
  const [renameHubModalOpen, setRenameHubModalOpen] = useState(false);
  const [hubToRename, setHubToRename] = useState<{
    id: string;
    name: string;
  } | null>(null);
  const [renameHubInput, setRenameHubInput] = useState('');
  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});
  const [hubDetails, setHubDetails] = useState<
    Record<string, { size: number; createTime: number }>
  >({});
  const [deleteHubsModalOpen, setDeleteHubsModalOpen] = useState(false);
  const [searchResults, setSearchResults] = useState<Skill[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [hasSearched, setHasSearched] = useState(false);

  // Selection state derived values (must be declared before any functions that use them)
  const selectedHubCount = useMemo(
    () => Object.keys(rowSelection).length,
    [rowSelection],
  );
  const selectedHubIds = useMemo(
    () => Object.keys(rowSelection),
    [rowSelection],
  );
  const hasSelectedHubs = selectedHubCount > 0;

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
    setRowSelection({}); // Clear selection when loading new data
    try {
      const nextHubs = await fetchHubs();
      setHubs(nextHubs);
      // Fetch folder details for each hub
      const details: Record<string, { size: number; createTime: number }> = {};
      for (const hub of nextHubs) {
        if (hub.folder_id) {
          try {
            const { data } = await fileManagerService.listFile({
              parent_id: hub.folder_id,
            });
            if (data.code === 0) {
              const files = data.data?.files || [];
              const totalSize = files.reduce(
                (sum: number, f: any) => sum + (f.size || 0),
                0,
              );
              details[hub.id] = {
                size: totalSize,
                createTime: hub.create_time || Date.now(),
              };
            }
          } catch (e) {
            console.warn('Failed to fetch hub folder details:', e);
          }
        }
      }
      setHubDetails(details);
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
    // Clear search results when switching hubs
    setSearchResults([]);
    setHasSearched(false);
    setSearchQuery('');
    fetchConfig(undefined, selectedHubId);
    // Use hub name for file system operations
    fetchSkills(selectedHubName);
  }, [
    selectedHubId,
    selectedHubName,
    fetchConfig,
    fetchSkills,
    setSearchQuery,
  ]);

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
      // Pass both hub name (for file system) and hub ID (for indexing)
      return await uploadSkill(
        name,
        version,
        files,
        selectedHubName,
        selectedHubId,
      );
    },
    [uploadSkill, selectedHubName, selectedHubId],
  );

  const handleDelete = useCallback(
    async (skillId: string, skillName: string, folderId?: string) => {
      // Pass both hub ID (for index), hub name (for file system), and folderId (for search results)
      const success = await deleteSkill(
        skillId,
        skillName,
        selectedHubId,
        selectedHubName,
        folderId,
      );
      // If delete succeeded and we have search results, remove the skill from searchResults
      if (success) {
        setSearchResults((prev) => prev.filter((s) => s.id !== skillId));
      }
    },
    [deleteSkill, selectedHubId, selectedHubName],
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

  const handleDeleteHub = useCallback(async () => {
    if (!hubToDelete) return;
    const success = await deleteHub(hubToDelete.id);
    if (success) {
      setDeleteHubModalOpen(false);
      setHubToDelete(null);
      await loadHubs();
    }
  }, [hubToDelete, deleteHub, loadHubs]);

  const openDeleteHubModal = useCallback(
    (hub: { id: string; name: string }, e: React.MouseEvent) => {
      e.stopPropagation();
      setHubToDelete(hub);
      setDeleteHubModalOpen(true);
    },
    [],
  );

  const openRenameHubModal = useCallback(
    (hub: { id: string; name: string }, e: React.MouseEvent) => {
      e.stopPropagation();
      setHubToRename(hub);
      setRenameHubInput(hub.name);
      setRenameHubModalOpen(true);
    },
    [],
  );

  const handleRenameHub = useCallback(async () => {
    if (!hubToRename || !renameHubInput.trim()) return;
    const success = await updateHub(hubToRename.id, renameHubInput.trim());
    if (success) {
      setRenameHubModalOpen(false);
      setHubToRename(null);
      setRenameHubInput('');
      await loadHubs();
      // Update selected hub name if it's the current hub
      if (selectedHubId === hubToRename.id) {
        setSelectedHubName(renameHubInput.trim());
      }
    }
  }, [hubToRename, renameHubInput, updateHub, loadHubs, selectedHubId]);

  const handleDeleteSelectedHubs = useCallback(async () => {
    let hasError = false;
    for (const hubId of selectedHubIds) {
      const success = await deleteHub(hubId);
      if (!success) {
        hasError = true;
      }
    }
    setDeleteHubsModalOpen(false);
    setRowSelection({});
    await loadHubs();
  }, [selectedHubIds, deleteHub, loadHubs]);

  const handleOpenDeleteSelectedModal = useCallback(() => {
    setDeleteHubsModalOpen(true);
  }, []);

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
          setSearchResults([]);
          setHasSearched(false);
          setSearchQuery('');
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
              <div className="flex items-center gap-2">
                <Segmented
                  value={hubViewMode}
                  onChange={(v) => setHubViewMode(v as 'grid' | 'list')}
                  options={[
                    { value: 'grid', label: <LayoutGrid className="size-4" /> },
                    { value: 'list', label: <List className="size-4" /> },
                  ]}
                />
                <Button onClick={() => setCreateHubModalOpen(true)}>
                  <Plus className="size-[1em]" />
                  {t('skills.createHub') || 'Create Skills Hub'}
                </Button>
              </div>
            </ListFilterBar>

            {hasSelectedHubs && hubViewMode === 'list' && (
              <BulkOperateBar
                className="mt-4"
                count={selectedHubCount}
                unit={t('skills.hub') || 'hubs'}
                list={[
                  {
                    id: 'delete',
                    label: t('common.delete'),
                    icon: <Trash2 className="size-4" />,
                    onClick: handleOpenDeleteSelectedModal,
                  },
                ]}
              />
            )}
          </header>

          <div className="flex-1 px-5 flex flex-col overflow-hidden">
            {hubLoading ? (
              <div className="flex-1 flex items-center justify-center">
                <Spin size="large" />
              </div>
            ) : filteredHubs.length ? (
              hubViewMode === 'grid' ? (
                <CardContainer className="flex-1 overflow-auto">
                  {filteredHubs.map((hub) => (
                    <div
                      key={hub.id}
                      className="group flex flex-col rounded-xl border border-border p-4 hover:border-accent-primary hover:shadow-md transition-all cursor-pointer bg-bg-card relative"
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
                        <div className="flex opacity-0 group-hover:opacity-100 transition-opacity">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8 text-text-secondary hover:text-accent-primary"
                            onClick={(e: React.MouseEvent) =>
                              openRenameHubModal(hub, e)
                            }
                          >
                            <Pencil className="size-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8 text-text-secondary hover:text-red-500"
                            onClick={(e: React.MouseEvent) =>
                              openDeleteHubModal(hub, e)
                            }
                          >
                            <Trash2 className="size-4" />
                          </Button>
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
                <div className="flex-1 overflow-auto border border-border rounded-lg">
                  <table className="w-full" style={{ tableLayout: 'fixed' }}>
                    <thead className="bg-bg-title sticky top-0">
                      <tr>
                        <th className="px-4 py-3 w-10">
                          <Checkbox
                            checked={
                              filteredHubs.length > 0 &&
                              filteredHubs.every((hub) => rowSelection[hub.id])
                            }
                            onCheckedChange={(checked) => {
                              const newSelection = { ...rowSelection };
                              filteredHubs.forEach((hub) => {
                                if (checked) {
                                  newSelection[hub.id] = true;
                                } else {
                                  delete newSelection[hub.id];
                                }
                              });
                              setRowSelection(newSelection);
                            }}
                          />
                        </th>
                        <th className="px-4 py-3 text-left text-sm font-medium text-text-title w-[20vw]">
                          {t('skills.hubName') || 'Name'}
                        </th>
                        <th className="px-4 py-3 text-left text-sm font-medium text-text-title w-40">
                          {t('fileManager.uploadDate') || 'Upload Date'}
                        </th>
                        <th className="px-4 py-3 text-left text-sm font-medium text-text-title w-24">
                          {t('fileManager.size') || 'Size'}
                        </th>
                        <th className="px-4 py-3 text-right text-sm font-medium text-text-title w-24">
                          {t('common.action') || 'Action'}
                        </th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-border">
                      {filteredHubs.map((hub) => (
                        <tr
                          key={hub.id}
                          className="hover:bg-bg-secondary/50 cursor-pointer transition-colors"
                          onClick={() => {
                            setSelectedHubId(hub.id);
                            setSelectedHubName(hub.name);
                          }}
                        >
                          <td
                            className="px-4 py-3"
                            onClick={(e) => e.stopPropagation()}
                          >
                            <Checkbox
                              checked={!!rowSelection[hub.id]}
                              onCheckedChange={(checked) => {
                                setRowSelection((prev) => {
                                  const newSelection = { ...prev };
                                  if (checked) {
                                    newSelection[hub.id] = true;
                                  } else {
                                    delete newSelection[hub.id];
                                  }
                                  return newSelection;
                                });
                              }}
                            />
                          </td>
                          <td className="px-4 py-3 w-[20vw]">
                            <div className="flex items-center gap-2 overflow-hidden">
                              <FolderOpen className="size-4 text-text-secondary flex-shrink-0" />
                              <span className="font-medium truncate">
                                {hub.name}
                              </span>
                            </div>
                          </td>
                          <td className="px-4 py-3 text-sm text-text-secondary">
                            {hubDetails[hub.id]?.createTime
                              ? formatDate(hubDetails[hub.id].createTime)
                              : '-'}
                          </td>
                          <td className="px-4 py-3 text-sm text-text-secondary">
                            {hubDetails[hub.id]?.size !== undefined
                              ? formatFileSize(hubDetails[hub.id].size)
                              : '-'}
                          </td>
                          <td
                            className="px-4 py-3 text-right"
                            onClick={(e) => e.stopPropagation()}
                          >
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8 text-text-secondary hover:text-accent-primary"
                              onClick={(e: React.MouseEvent) =>
                                openRenameHubModal(hub, e)
                              }
                            >
                              <Pencil className="size-4" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8 text-text-secondary hover:text-red-500"
                              onClick={(e: React.MouseEvent) =>
                                openDeleteHubModal(hub, e)
                              }
                            >
                              <Trash2 className="size-4" />
                            </Button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )
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

        {/* Delete Hub Modal */}
        <Dialog open={deleteHubModalOpen} onOpenChange={setDeleteHubModalOpen}>
          <DialogContent className="sm:max-w-[425px]">
            <DialogHeader>
              <DialogTitle>
                {t('skills.deleteHubTitle') || 'Delete Skills Hub'}
              </DialogTitle>
              <DialogDescription>
                {t('skills.deleteHubDescription') ||
                  'Are you sure you want to delete this skills hub? This action cannot be undone and all skills in this hub will be permanently deleted.'}
              </DialogDescription>
            </DialogHeader>
            <div className="py-4">
              <p className="text-sm text-text-secondary">
                {t('skills.deleteHubName') || 'Hub name'}:{' '}
                <strong>{hubToDelete?.name}</strong>
              </p>
            </div>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => {
                  setDeleteHubModalOpen(false);
                  setHubToDelete(null);
                }}
              >
                {t('common.cancel')}
              </Button>
              <Button variant="destructive" onClick={handleDeleteHub}>
                {t('common.delete')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Rename Hub Modal */}
        <Dialog open={renameHubModalOpen} onOpenChange={setRenameHubModalOpen}>
          <DialogContent className="sm:max-w-[425px]">
            <DialogHeader>
              <DialogTitle>
                {t('skills.renameHubTitle') || 'Rename Skills Hub'}
              </DialogTitle>
              <DialogDescription>
                {t('skills.renameHubDescription') ||
                  'Enter a new name for this skills hub.'}
              </DialogDescription>
            </DialogHeader>
            <div className="py-4">
              <label className="text-sm font-medium mb-2 block">
                {t('skills.hubName') || 'Hub Name'}
              </label>
              <Input
                placeholder={t('skills.hubNamePlaceholder') || 'e.g., my-hub'}
                value={renameHubInput}
                onChange={(e) => setRenameHubInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && renameHubInput.trim()) {
                    handleRenameHub();
                  }
                }}
              />
            </div>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => {
                  setRenameHubModalOpen(false);
                  setHubToRename(null);
                  setRenameHubInput('');
                }}
              >
                {t('common.cancel')}
              </Button>
              <Button
                onClick={handleRenameHub}
                disabled={
                  !renameHubInput.trim() ||
                  renameHubInput.trim() === hubToRename?.name
                }
              >
                {t('common.save') || 'Save'}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Delete Selected Hubs Modal */}
        <Dialog
          open={deleteHubsModalOpen}
          onOpenChange={setDeleteHubsModalOpen}
        >
          <DialogContent className="sm:max-w-[425px]">
            <DialogHeader>
              <DialogTitle>
                {t('skills.deleteSelectedHubsTitle') ||
                  'Delete Selected Skills Hubs'}
              </DialogTitle>
              <DialogDescription>
                {t('skills.deleteSelectedHubsDescription') ||
                  'Are you sure you want to delete the selected skills hubs? This action cannot be undone and all skills in these hubs will be permanently deleted.'}
              </DialogDescription>
            </DialogHeader>
            <div className="py-4">
              <p className="text-sm text-text-secondary">
                {t('skills.selectedHubsCount') || 'Selected hubs'}:{' '}
                <strong>{selectedHubCount}</strong>
              </p>
            </div>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => {
                  setDeleteHubsModalOpen(false);
                }}
              >
                {t('common.cancel')}
              </Button>
              <Button variant="destructive" onClick={handleDeleteSelectedHubs}>
                {t('common.delete')}
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
                    onClick={() => fetchSkills(selectedHubName)}
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
