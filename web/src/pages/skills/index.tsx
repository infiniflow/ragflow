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
  const [spaces, setSpaces] = useState<Array<{ id: string; name: string }>>([]);
  const [spaceInput, setSpaceInput] = useState('');
  const [selectedSpaceId, setSelectedSpaceId] = useState<string>('');
  const [selectedSpaceName, setSelectedSpaceName] = useState<string>('');
  const [spaceLoading, setSpaceLoading] = useState(false);
  const [spaceSearchString, setSpaceSearchString] = useState('');

  const {
    skills,
    filteredSkills,
    loading,
    searchQuery,
    setSearchQuery,
    fetchSpaces,
    createSpace,
    deleteSpace,
    updateSpace,
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
  } = useSkillSearchConfig(selectedSpaceId);

  const [viewMode, setViewMode] = useState<'grid' | 'list'>('grid');
  const [spaceViewMode, setSpaceViewMode] = useState<'grid' | 'list'>('grid');
  const [detailOpen, setDetailOpen] = useState(false);
  const [selectedSkill, setSelectedSkill] = useState<Skill | null>(null);
  const [uploadModalOpen, setUploadModalOpen] = useState(false);
  const [configModalOpen, setConfigModalOpen] = useState(false);
  const [createSpaceModalOpen, setCreateSpaceModalOpen] = useState(false);
  const [deleteSpaceModalOpen, setDeleteSpaceModalOpen] = useState(false);
  const [spaceToDelete, setSpaceToDelete] = useState<{
    id: string;
    name: string;
  } | null>(null);
  const [renameSpaceModalOpen, setRenameSpaceModalOpen] = useState(false);
  const [spaceToRename, setSpaceToRename] = useState<{
    id: string;
    name: string;
  } | null>(null);
  const [renameSpaceInput, setRenameSpaceInput] = useState('');
  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});
  const [spaceDetails, setSpaceDetails] = useState<
    Record<string, { size: number; createTime: number }>
  >({});
  const [deleteSpacesModalOpen, setDeleteSpacesModalOpen] = useState(false);
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
  const selectedSpaceCount = useMemo(
    () => Object.keys(rowSelection).length,
    [rowSelection],
  );
  const selectedSpaceIds = useMemo(
    () => Object.keys(rowSelection),
    [rowSelection],
  );
  const hasSelectedSpaces = selectedSpaceCount > 0;

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

  const loadSpaces = useCallback(async () => {
    setSpaceLoading(true);
    setRowSelection({}); // Clear selection when loading new data
    try {
      const nextSpaces = await fetchSpaces();
      setSpaces(nextSpaces);
      // Fetch folder details for each space
      const details: Record<string, { size: number; createTime: number }> = {};
      for (const space of nextSpaces) {
        if (space.folder_id) {
          try {
            const { data } = await fileManagerService.listFile({
              parent_id: space.folder_id,
            });
            if (data.code === 0) {
              const files = data.data?.files || [];
              const totalSize = files.reduce(
                (sum: number, f: any) => sum + (f.size || 0),
                0,
              );
              details[space.id] = {
                size: totalSize,
                createTime: space.create_time || Date.now(),
              };
            }
          } catch (e) {
            console.warn('Failed to fetch space folder details:', e);
          }
        }
      }
      setSpaceDetails(details);
    } finally {
      setSpaceLoading(false);
    }
  }, [fetchSpaces]);

  useEffect(() => {
    loadSpaces();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Function to load skills with pagination and sorting
  const loadSkills = useCallback(async () => {
    const result = await fetchSkills(
      selectedSpaceName,
      selectedSpaceId,
      currentPage,
      pageSize,
      sortBy,
      sortOrder,
    );
    setTotalSkills(result.total);
  }, [
    fetchSkills,
    selectedSpaceName,
    selectedSpaceId,
    currentPage,
    pageSize,
    sortBy,
    sortOrder,
  ]);

  // Load skills when space changes or pagination/sorting changes
  useEffect(() => {
    if (!selectedSpaceId || !selectedSpaceName) return;
    // Clear search results when switching spaces
    setSearchResults([]);
    setHasSearched(false);
    setSearchQuery('');
    setCurrentPage(1);
    fetchConfig(undefined, selectedSpaceId);
    // Use search API with pagination and sorting
    loadSkills();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedSpaceId, selectedSpaceName]);

  // Load skills when pagination or sorting changes
  useEffect(() => {
    if (!selectedSpaceId || !selectedSpaceName || hasSearched) return;
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
      // Pass space name (for file system), space ID (for indexing), and embd_id (for indexing)
      return await uploadSkill(
        name,
        version,
        files,
        selectedSpaceName,
        selectedSpaceId,
        config?.embd_id,
      );
    },
    [uploadSkill, selectedSpaceName, selectedSpaceId, config?.embd_id],
  );

  const handleDelete = useCallback(
    async (skillId: string, skillName: string, folderId?: string) => {
      // Pass both space ID (for index), space name (for file system), and folderId (for search results)
      const success = await deleteSkill(
        skillId,
        skillName,
        selectedSpaceId,
        selectedSpaceName,
        folderId,
      );
      // If delete succeeded and we have search results, remove the skill from searchResults
      if (success) {
        setSearchResults((prev) => prev.filter((s) => s.id !== skillId));
      }
    },
    [deleteSkill, selectedSpaceId, selectedSpaceName],
  );

  const handleCreateHub = useCallback(async () => {
    const nextHubName = spaceInput.trim();
    if (!nextHubName) return;
    const newHub = await createSpace(nextHubName);
    if (!newHub) return;
    setSpaceInput('');
    setCreateSpaceModalOpen(false);
    await loadSpaces();
    // Select the newly created space
    setSelectedSpaceId(newHub.id);
    setSelectedSpaceName(newHub.name);
  }, [spaceInput, createSpace, loadSpaces]);

  const handleDeleteHub = useCallback(async () => {
    if (!spaceToDelete) return;
    const success = await deleteSpace(spaceToDelete.id);
    if (success) {
      setDeleteSpaceModalOpen(false);
      setSpaceToDelete(null);
      await loadSpaces();
    }
  }, [spaceToDelete, deleteSpace, loadSpaces]);

  const openDeleteSpaceModal = useCallback(
    (space: { id: string; name: string }, e: React.MouseEvent) => {
      e.stopPropagation();
      setSpaceToDelete(space);
      setDeleteSpaceModalOpen(true);
    },
    [],
  );

  const openRenameSpaceModal = useCallback(
    (space: { id: string; name: string }, e: React.MouseEvent) => {
      e.stopPropagation();
      setSpaceToRename(space);
      setRenameSpaceInput(space.name);
      setRenameSpaceModalOpen(true);
    },
    [],
  );

  const handleRenameHub = useCallback(async () => {
    if (!spaceToRename || !renameSpaceInput.trim()) return;
    const success = await updateSpace(
      spaceToRename.id,
      renameSpaceInput.trim(),
    );
    if (success) {
      setRenameSpaceModalOpen(false);
      setSpaceToRename(null);
      setRenameSpaceInput('');
      await loadSpaces();
      // Update selected space name if it's the current space
      if (selectedSpaceId === spaceToRename.id) {
        setSelectedSpaceName(renameSpaceInput.trim());
      }
    }
  }, [
    spaceToRename,
    renameSpaceInput,
    updateSpace,
    loadSpaces,
    selectedSpaceId,
  ]);

  const handleDeleteSelectedHubs = useCallback(async () => {
    for (const hubId of selectedSpaceIds) {
      await deleteSpace(hubId);
    }
    setDeleteSpacesModalOpen(false);
    setRowSelection({});
    await loadSpaces();
  }, [selectedSpaceIds, deleteSpace, loadSpaces]);

  const handleOpenDeleteSelectedModal = useCallback(() => {
    setDeleteSpacesModalOpen(true);
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
      setSpaceSearchString(e.target.value);
    },
    [],
  );

  const filteredSpaces = useMemo(() => {
    if (!spaceSearchString.trim()) return spaces;
    return spaces.filter((space) =>
      space.name.toLowerCase().includes(spaceSearchString.toLowerCase()),
    );
  }, [spaces, spaceSearchString]);

  const displayedSkills = useMemo(() => {
    // Server-side sorting is already applied via API, no need to sort here
    return hasSearched ? searchResults : filteredSkills;
  }, [hasSearched, searchResults, filteredSkills]);

  const isLoading = loading || isSearching || configLoading;

  // Space list breadcrumb: root / skills
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
          setSelectedSpaceId('');
          setSelectedSpaceName('');
          setSearchResults([]);
          setHasSearched(false);
          setSearchQuery('');
          fetchSkills(''); // Clear skills data
        }}
      >
        {t('skills.title')}
      </span>
      <span className="text-text-secondary">/</span>
      <span>{selectedSpaceName}</span>
    </div>
  );

  // Space list page (no space selected)
  if (!selectedSpaceId) {
    return (
      <>
        <article
          className="size-full flex flex-col"
          data-testid="skill-space-list"
        >
          <header className="px-5 pt-8 mb-4">
            <ListFilterBar
              leftPanel={hubListBreadcrumb}
              searchString={spaceSearchString}
              onSearchChange={handleHubSearchChange}
              showFilter={false}
              icon="file"
            >
              <div className="flex items-center gap-2">
                <Segmented
                  value={spaceViewMode}
                  onChange={(v) => setSpaceViewMode(v as 'grid' | 'list')}
                  options={[
                    { value: 'grid', label: <LayoutGrid className="size-4" /> },
                    { value: 'list', label: <List className="size-4" /> },
                  ]}
                />
                <Button onClick={() => setCreateSpaceModalOpen(true)}>
                  <Plus className="size-[1em]" />
                  {t('skills.createSpace') || 'Create Skill Space'}
                </Button>
              </div>
            </ListFilterBar>

            {hasSelectedSpaces && spaceViewMode === 'list' && (
              <BulkOperateBar
                className="mt-4"
                count={selectedSpaceCount}
                unit={t('skills.space') || 'spaces'}
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
            {spaceLoading ? (
              <div className="flex-1 flex items-center justify-center">
                <Spin size="large" />
              </div>
            ) : filteredSpaces.length ? (
              spaceViewMode === 'grid' ? (
                <CardContainer className="flex-1 overflow-auto">
                  {filteredSpaces.map((space) => (
                    <div
                      key={space.id}
                      className="group flex flex-col rounded-xl border border-border p-4 hover:border-accent-primary hover:shadow-md transition-all cursor-pointer bg-bg-card relative"
                      onClick={() => {
                        setSelectedSpaceId(space.id);
                        setSelectedSpaceName(space.name);
                      }}
                    >
                      <div className="flex items-start justify-between mb-2">
                        <div className="flex-1 min-w-0 flex items-center gap-2">
                          <SvgIcon
                            name="home-icon/skill-space"
                            width={20}
                            height={20}
                          />
                          <h3 className="font-semibold text-lg truncate">
                            {space.name}
                          </h3>
                        </div>
                        <div className="flex opacity-0 group-hover:opacity-100 transition-opacity">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8 text-text-secondary hover:text-accent-primary"
                            onClick={(e: React.MouseEvent) =>
                              openRenameSpaceModal(space, e)
                            }
                          >
                            <Pencil className="size-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8 text-text-secondary hover:text-red-500"
                            onClick={(e: React.MouseEvent) =>
                              openDeleteSpaceModal(space, e)
                            }
                          >
                            <Trash2 className="size-4" />
                          </Button>
                        </div>
                      </div>
                      <div className="mt-auto pt-2">
                        <span className="text-accent-primary text-sm">
                          {t('skills.enterSpace') || 'Enter'} →
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
                              filteredSpaces.length > 0 &&
                              filteredSpaces.every(
                                (space) => rowSelection[space.id],
                              )
                            }
                            onCheckedChange={(checked) => {
                              const newSelection = { ...rowSelection };
                              filteredSpaces.forEach((space) => {
                                if (checked) {
                                  newSelection[space.id] = true;
                                } else {
                                  delete newSelection[space.id];
                                }
                              });
                              setRowSelection(newSelection);
                            }}
                          />
                        </th>
                        <th className="px-4 py-3 text-left text-sm font-medium text-text-title">
                          {t('skills.spaceName') || 'Name'}
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
                      {filteredSpaces.map((space) => (
                        <tr
                          key={space.id}
                          className="hover:bg-bg-secondary/50 cursor-pointer transition-colors"
                          onClick={() => {
                            setSelectedSpaceId(space.id);
                            setSelectedSpaceName(space.name);
                          }}
                        >
                          <td
                            className="px-3 py-3 text-center"
                            onClick={(e) => e.stopPropagation()}
                          >
                            <Checkbox
                              checked={!!rowSelection[space.id]}
                              onCheckedChange={(checked) => {
                                setRowSelection((prev) => {
                                  const newSelection = { ...prev };
                                  if (checked) {
                                    newSelection[space.id] = true;
                                  } else {
                                    delete newSelection[space.id];
                                  }
                                  return newSelection;
                                });
                              }}
                            />
                          </td>
                          <td className="px-4 py-3">
                            <div className="flex items-center gap-2 overflow-hidden">
                              <SvgIcon
                                name="home-icon/skill-space"
                                width={16}
                                height={16}
                              />
                              <span className="font-medium truncate">
                                {space.name}
                              </span>
                            </div>
                          </td>
                          <td className="px-4 py-3 text-sm text-text-secondary">
                            {spaceDetails[space.id]?.createTime
                              ? formatDate(spaceDetails[space.id].createTime)
                              : '-'}
                          </td>
                          <td className="px-4 py-3 text-sm text-text-secondary">
                            {spaceDetails[space.id]?.size !== undefined
                              ? formatFileSize(spaceDetails[space.id].size)
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
                                openRenameSpaceModal(space, e)
                              }
                            >
                              <Pencil className="size-4" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8 text-text-secondary hover:text-red-500"
                              onClick={(e: React.MouseEvent) =>
                                openDeleteSpaceModal(space, e)
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
                {spaceSearchString ? (
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
                    onClick={() => setCreateSpaceModalOpen(true)}
                  />
                )}
              </div>
            )}
          </div>
        </article>

        {/* Create Space Modal */}
        <Dialog
          open={createSpaceModalOpen}
          onOpenChange={setCreateSpaceModalOpen}
        >
          <DialogContent className="sm:max-w-[425px]">
            <DialogHeader>
              <DialogTitle>
                {t('skills.createSpaceTitle') || 'Create New Skill Space'}
              </DialogTitle>
              <DialogDescription>
                {t('skills.createSpaceDescription') ||
                  'Create a new space to organize and manage your skills.'}
              </DialogDescription>
            </DialogHeader>
            <div className="py-4">
              <label className="text-sm font-medium mb-2 block">
                {t('skills.spaceName') || 'Space Name'}
              </label>
              <Input
                placeholder={
                  t('skills.spaceNamePlaceholder') || 'e.g., my-space'
                }
                value={spaceInput}
                onChange={(e) => setSpaceInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && spaceInput.trim()) {
                    handleCreateHub();
                  }
                }}
              />
            </div>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => {
                  setCreateSpaceModalOpen(false);
                  setSpaceInput('');
                }}
              >
                {t('common.cancel')}
              </Button>
              <Button onClick={handleCreateHub} disabled={!spaceInput.trim()}>
                {t('common.create')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Delete Space Modal */}
        <Dialog
          open={deleteSpaceModalOpen}
          onOpenChange={setDeleteSpaceModalOpen}
        >
          <DialogContent className="sm:max-w-[425px]">
            <DialogHeader>
              <DialogTitle>
                {t('skills.deleteSpaceTitle') || 'Delete Skill Space'}
              </DialogTitle>
              <DialogDescription>
                {t('skills.deleteSpaceDescription') ||
                  'Are you sure you want to delete this skill space? This action cannot be undone and all skills in this space will be permanently deleted.'}
              </DialogDescription>
            </DialogHeader>
            <div className="py-4">
              <p className="text-sm text-text-secondary">
                {t('skills.deleteSpaceName') || 'Space name'}:{' '}
                <strong>{spaceToDelete?.name}</strong>
              </p>
            </div>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => {
                  setDeleteSpaceModalOpen(false);
                  setSpaceToDelete(null);
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

        {/* Rename Space Modal */}
        <Dialog
          open={renameSpaceModalOpen}
          onOpenChange={setRenameSpaceModalOpen}
        >
          <DialogContent className="sm:max-w-[425px]">
            <DialogHeader>
              <DialogTitle>
                {t('skills.renameSpaceTitle') || 'Rename Skill Space'}
              </DialogTitle>
              <DialogDescription>
                {t('skills.renameSpaceDescription') ||
                  'Enter a new name for this skill space.'}
              </DialogDescription>
            </DialogHeader>
            <div className="py-4">
              <label className="text-sm font-medium mb-2 block">
                {t('skills.spaceName') || 'Space Name'}
              </label>
              <Input
                placeholder={
                  t('skills.spaceNamePlaceholder') || 'e.g., my-space'
                }
                value={renameSpaceInput}
                onChange={(e) => setRenameSpaceInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && renameSpaceInput.trim()) {
                    handleRenameHub();
                  }
                }}
              />
            </div>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => {
                  setRenameSpaceModalOpen(false);
                  setSpaceToRename(null);
                  setRenameSpaceInput('');
                }}
              >
                {t('common.cancel')}
              </Button>
              <Button
                onClick={handleRenameHub}
                disabled={
                  !renameSpaceInput.trim() ||
                  renameSpaceInput.trim() === spaceToRename?.name
                }
              >
                {t('common.save') || 'Save'}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Delete Selected Hubs Modal */}
        <Dialog
          open={deleteSpacesModalOpen}
          onOpenChange={setDeleteSpacesModalOpen}
        >
          <DialogContent className="sm:max-w-[425px]">
            <DialogHeader>
              <DialogTitle>
                {t('skills.deleteSelectedHubsTitle') ||
                  'Delete Selected Skills Hubs'}
              </DialogTitle>
              <DialogDescription>
                {t('skills.deleteSelectedHubsDescription') ||
                  'Are you sure you want to delete the selected skills spaces? This action cannot be undone and all skills in these spaces will be permanently deleted.'}
              </DialogDescription>
            </DialogHeader>
            <div className="py-4">
              <p className="text-sm text-text-secondary">
                {t('skills.selectedHubsCount') || 'Selected spaces'}:{' '}
                <strong>{selectedSpaceCount}</strong>
              </p>
            </div>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => {
                  setDeleteSpacesModalOpen(false);
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

  // Inside a space (skills list page)
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
