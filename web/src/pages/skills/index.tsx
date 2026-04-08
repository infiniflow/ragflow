import { BulkOperateBar } from '@/components/bulk-operate-bar';
import { CardContainer } from '@/components/card-container';
import { EmptyCardType } from '@/components/empty/constant';
import { EmptyAppCard } from '@/components/empty/empty';
import ListFilterBar from '@/components/list-filter-bar';
import SvgIcon from '@/components/svg-icon';
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
  Eye,
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
    getSkillDetails,
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
  const [skillDetailLoading, setSkillDetailLoading] = useState(false);

  // Pagination and sorting state
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize] = useState(20);
  const [totalSkills, setTotalSkills] = useState(0);
  const [sortBy, setSortBy] = useState<'name' | 'update_time' | 'create_time'>(
    'update_time',
  );
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc');

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

  // Function to load skills with pagination and sorting
  const loadSkills = useCallback(async () => {
    const result = await fetchSkills(
      selectedHubName,
      selectedHubId,
      currentPage,
      pageSize,
      sortBy,
      sortOrder,
    );
    setTotalSkills(result.total);
  }, [
    fetchSkills,
    selectedHubName,
    selectedHubId,
    currentPage,
    pageSize,
    sortBy,
    sortOrder,
  ]);

  // Load skills when hub changes or pagination/sorting changes
  useEffect(() => {
    if (!selectedHubId || !selectedHubName) return;
    // Clear search results when switching hubs
    setSearchResults([]);
    setHasSearched(false);
    setSearchQuery('');
    setCurrentPage(1);
    fetchConfig(undefined, selectedHubId);
    // Use search API with pagination and sorting
    loadSkills();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedHubId, selectedHubName]);

  // Load skills when pagination or sorting changes
  useEffect(() => {
    if (!selectedHubId || !selectedHubName || hasSearched) return;
    loadSkills();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentPage, sortBy, sortOrder]);

  const handleViewSkill = useCallback(
    async (skill: Skill) => {
      // If skill already has versions, use it directly
      if (skill.versions && skill.versions.length > 0) {
        setSelectedSkill(skill);
        setDetailOpen(true);
        return;
      }

      // Try to enrich skill data with versions from existing skills list
      if (!(skill as any)._folderId || !skill.versions) {
        const existingSkill = filteredSkills.find((s) => s.id === skill.id);
        if (existingSkill) {
          if ((existingSkill as any)._folderId) {
            skill = {
              ...skill,
              _folderId: (existingSkill as any)._folderId,
            };
          }
          if (existingSkill.versions && existingSkill.versions.length > 0) {
            skill = {
              ...skill,
              versions: existingSkill.versions,
              files: existingSkill.files,
            };
          }
        }
      }

      // If still no versions but has folderId, fetch from file system
      if (
        (!skill.versions || skill.versions.length === 0) &&
        (skill as any)._folderId
      ) {
        setSkillDetailLoading(true);
        try {
          const detailedSkill = await getSkillDetails(
            (skill as any)._folderId,
            skill.name,
          );
          if (detailedSkill) {
            skill = {
              ...skill,
              versions: detailedSkill.versions,
              files: detailedSkill.files,
              metadata: {
                ...skill.metadata,
                ...detailedSkill.metadata,
              },
            };
          }
        } catch (error) {
          console.warn('Failed to fetch skill details:', error);
        } finally {
          setSkillDetailLoading(false);
        }
      }

      if (!(skill as any)._folderId) {
        console.warn(
          `[Skill Search] Skill "${skill.name}" has no folder_id. ` +
            'Please reindex skills to fix this issue.',
        );
      }

      setSelectedSkill(skill);
      setDetailOpen(true);
    },
    [filteredSkills, getSkillDetails],
  );

  const handleCloseDetail = useCallback(() => {
    setDetailOpen(false);
    setSelectedSkill(null);
  }, []);

  const handleUpload = useCallback(
    async (name: string, version: string, files: File[]) => {
      // Pass hub name (for file system), hub ID (for indexing), and embd_id (for indexing)
      return await uploadSkill(
        name,
        version,
        files,
        selectedHubName,
        selectedHubId,
        config?.embd_id,
      );
    },
    [uploadSkill, selectedHubName, selectedHubId, config?.embd_id],
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
    for (const hubId of selectedHubIds) {
      await deleteHub(hubId);
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
              versions: localSkill.versions,
              files: localSkill.files,
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
    // Server-side sorting is already applied via API, no need to sort here
    return hasSearched ? searchResults : filteredSkills;
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
          fetchSkills(''); // Clear skills data
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
              icon="file"
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
                        <div className="flex-1 min-w-0 flex items-center gap-2">
                          <SvgIcon
                            name="home-icon/skills-hub"
                            width={20}
                            height={20}
                          />
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
                    <colgroup>
                      <col style={{ width: '50px' }} />
                      <col style={{ width: '20vw' }} />
                      <col style={{ width: '160px' }} />
                      <col style={{ width: '96px' }} />
                      <col style={{ width: '96px' }} />
                    </colgroup>
                    <thead className="bg-bg-title sticky top-0">
                      <tr>
                        <th className="px-3 py-3 text-center">
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
                        <th className="px-4 py-3 text-left text-sm font-medium text-text-title">
                          {t('skills.hubName') || 'Name'}
                        </th>
                        <th className="px-4 py-3 text-left text-sm font-medium text-text-title">
                          {t('fileManager.uploadDate') || 'Upload Date'}
                        </th>
                        <th className="px-4 py-3 text-left text-sm font-medium text-text-title">
                          {t('fileManager.size') || 'Size'}
                        </th>
                        <th className="px-4 py-3 text-right text-sm font-medium text-text-title">
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
                            className="px-3 py-3 text-center"
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
                          <td className="px-4 py-3">
                            <div className="flex items-center gap-2 overflow-hidden">
                              <SvgIcon
                                name="home-icon/skills-hub"
                                width={16}
                                height={16}
                              />
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
          showSearch={false}
          icon="file"
        >
          <div className="flex items-center gap-2">
            {/* Search skills */}
            <div className="relative">
              <Input
                placeholder={
                  t('skills.searchPlaceholder') || 'Search skills...'
                }
                value={searchQuery}
                onChange={handleSearchInputChange}
                onKeyDown={handleSearchKeyDown}
                className="w-[200px] pr-10"
              />
              <button
                onClick={handleSearchClick}
                disabled={isSearching}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-text-secondary hover:text-text-primary disabled:opacity-50 disabled:cursor-not-allowed"
              >
                <Search className="size-4" />
              </button>
            </div>
            {/* Sort order toggle */}
            <Button
              variant="outline"
              size="icon"
              onClick={() => setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc')}
              title={
                sortOrder === 'asc'
                  ? t('skills.sortDesc') || 'Sort Descending'
                  : t('skills.sortAsc') || 'Sort Ascending'
              }
            >
              {sortOrder === 'asc' ? (
                <svg
                  className="size-4"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                >
                  <path d="M12 5v14M5 12l7-7 7 7" />
                </svg>
              ) : (
                <svg
                  className="size-4"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                >
                  <path d="M12 19V5M5 12l7 7 7-7" />
                </svg>
              )}
            </Button>

            {/* Grid/List toggle */}
            <Segmented
              value={viewMode}
              onChange={(v) => setViewMode(v as 'grid' | 'list')}
              options={[
                { value: 'grid', label: <LayoutGrid className="size-4" /> },
                { value: 'list', label: <List className="size-4" /> },
              ]}
            />
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
                    onClick={() => loadSkills()}
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
        ) : viewMode === 'grid' ? (
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
        ) : (
          <div className="flex-1 overflow-auto border border-border rounded-lg">
            <table className="w-full" style={{ tableLayout: 'fixed' }}>
              <colgroup>
                <col style={{ width: 'auto' }} />
                <col style={{ width: '120px' }} />
                <col style={{ width: '96px' }} />
              </colgroup>
              <thead className="bg-bg-title sticky top-0">
                <tr>
                  <th className="px-4 py-3 text-left text-sm font-medium text-text-title">
                    {t('skills.skillName') || 'Name'}
                  </th>
                  <th className="px-4 py-3 text-left text-sm font-medium text-text-title">
                    {t('skills.version') || 'Version'}
                  </th>
                  <th className="px-4 py-3 text-right text-sm font-medium text-text-title">
                    {t('common.action') || 'Action'}
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {displayedSkills.map((skill) => (
                  <tr
                    key={skill.id}
                    className="hover:bg-bg-secondary/50 cursor-pointer transition-colors"
                    onClick={() => handleViewSkill(skill)}
                  >
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2 overflow-hidden">
                        <SvgIcon
                          name="home-icon/skill-folder"
                          width={16}
                          height={16}
                        />
                        <span className="font-medium truncate">
                          {skill.name}
                        </span>
                      </div>
                      {skill.description && (
                        <p className="text-text-secondary text-xs mt-1 truncate">
                          {skill.description}
                        </p>
                      )}
                    </td>
                    <td className="px-4 py-3 text-sm text-text-secondary">
                      {skill.metadata?.version || '-'}
                    </td>
                    <td
                      className="px-4 py-3 text-right"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-text-secondary hover:text-accent-primary"
                        onClick={(e: React.MouseEvent) => {
                          e.stopPropagation();
                          handleViewSkill(skill);
                        }}
                      >
                        <Eye className="size-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-text-secondary hover:text-red-500"
                        onClick={(e: React.MouseEvent) => {
                          e.stopPropagation();
                          handleDelete(
                            skill.id,
                            skill.name,
                            (skill as any)._folderId,
                          );
                        }}
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {/* Pagination */}
        {!hasSearched && totalSkills > 0 && (
          <div className="flex items-center justify-between py-4 border-t border-border mt-4">
            <div className="text-sm text-text-secondary">
              {t('skills.totalSkills', { total: totalSkills })}
            </div>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={currentPage <= 1 || loading}
                onClick={() => setCurrentPage((p) => Math.max(1, p - 1))}
              >
                {t('common.previous')}
              </Button>
              <span className="text-sm text-text-secondary px-2">
                {t('skills.pageInfo', {
                  current: currentPage,
                  total: Math.ceil(totalSkills / pageSize),
                })}
              </span>
              <Button
                variant="outline"
                size="sm"
                disabled={
                  currentPage >= Math.ceil(totalSkills / pageSize) || loading
                }
                onClick={() => setCurrentPage((p) => p + 1)}
              >
                {t('common.next')}
              </Button>
            </div>
          </div>
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

      {/* Skill Detail Loading Overlay */}
      {skillDetailLoading && (
        <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/20">
          <Spin size="large" />
        </div>
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
