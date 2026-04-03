import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Spin } from '@/components/ui/spin';
import { TreeDataItem, TreeView } from '@/components/ui/tree-view';
import {
  ArrowBigLeft,
  FileCode,
  FileText,
  FolderOpen,
  Tag,
} from 'lucide-react';
import React, { memo, useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { isMarkdownFile } from '../hooks';
import type { Skill, SkillFileEntry } from '../types';
import CodeViewer from './code-viewer';
import MarkdownViewer from './markdown-viewer';

interface SkillDetailProps {
  skill: Skill | null;
  open: boolean;
  onClose: () => void;
  getFileContent: (
    skillId: string,
    filePath: string,
    version?: string,
    skillObj?: Skill,
  ) => Promise<string | null>;
  getVersionFiles?: (
    skillId: string,
    version: string,
    skillObj?: Skill,
  ) => Promise<SkillFileEntry[]>;
}

const getFileIcon = (filename: string, isDir: boolean) => {
  if (isDir) return FolderOpen;
  if (isMarkdownFile(filename)) return FileCode;
  return FileText;
};

// Build tree from flat file list
const buildFileTree = (files: SkillFileEntry[]): TreeDataItem[] => {
  const root: TreeDataItem[] = [];
  const map: Record<string, TreeDataItem> = {};

  // Sort files: directories first, then alphabetically
  const sortedFiles = [...files].sort((a, b) => {
    if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;
    return a.name.localeCompare(b.name);
  });

  sortedFiles.forEach((file) => {
    const parts = file.path.split('/');
    const name = parts[parts.length - 1];

    const node: TreeDataItem = {
      name: name,
      id: file.path,
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
  getVersionFiles,
}) => {
  const { t } = useTranslation();
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const [fileContent, setFileContent] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [selectedVersion, setSelectedVersion] = useState<string>('');
  const [versionFiles, setVersionFiles] = useState<SkillFileEntry[]>([]);
  const [versionLoading, setVersionLoading] = useState(false);

  // Check if skill has multiple versions
  const hasVersions = skill?.versions && skill.versions.length > 0;
  const availableVersions = skill?.versions || [];

  // Reset state when skill changes or drawer opens/closes
  useEffect(() => {
    if (open && skill) {
      // Initialize version
      if (hasVersions) {
        const defaultVersion = skill.metadata?.version || availableVersions[0];
        setSelectedVersion(defaultVersion);
      } else {
        setSelectedVersion('');
      }
    } else {
      // Reset when closed
      setSelectedVersion('');
      setVersionFiles([]);
      setVersionLoading(false);
      setSelectedFile(null);
      setFileContent('');
    }
  }, [
    open,
    skill?.id,
    hasVersions,
    skill?.metadata?.version,
    availableVersions,
  ]);

  const resolvedVersion = useMemo(() => {
    if (!skill) return '';
    return (
      selectedVersion || skill.metadata?.version || skill.versions?.[0] || ''
    );
  }, [selectedVersion, skill?.id, skill?.metadata?.version, skill?.versions]);

  // Load files when version or skill changes
  useEffect(() => {
    let isActive = true;

    const loadVersionFiles = async () => {
      if (!skill || !getVersionFiles) {
        if (isActive) {
          setVersionFiles([]);
          setVersionLoading(false);
        }
        return;
      }

      // Check if skill has _folderId (required for file operations)
      if (!(skill as any)._folderId) {
        console.warn(
          `[Skill Detail] Skill "${skill.name}" has no folder_id. ` +
            'Please reindex skills in settings to fix this issue.',
        );
        if (isActive) {
          setVersionFiles([]);
          setVersionLoading(false);
        }
        return;
      }

      // If it's the default version and skill.files is not empty, use skill.files
      // Only for local skills (not search results which have empty files array)
      if (
        resolvedVersion ===
          (skill.metadata?.version || skill.versions?.[0] || '') &&
        skill.files.length > 0 &&
        skill.source_type !== 'search'
      ) {
        if (isActive) {
          setVersionFiles(skill.files);
          setVersionLoading(false);
        }
        return;
      }

      // Load files for the selected version
      if (isActive) setVersionLoading(true);
      try {
        const versionToLoad = resolvedVersion;
        // Pass skill object to handle search results not in skills state
        const files = await getVersionFiles(skill.id, versionToLoad, skill);
        if (isActive) setVersionFiles(files);
      } catch (error) {
        console.error('Failed to load version files:', error);
        if (isActive) setVersionFiles([]);
      } finally {
        if (isActive) setVersionLoading(false);
      }
    };

    loadVersionFiles();

    return () => {
      isActive = false;
    };
  }, [
    skill?.id,
    skill?.source_type,
    skill?.metadata?.version,
    skill?.versions,
    (skill as any)?._folderId,
    skill?.files,
    resolvedVersion,
    getVersionFiles,
  ]);

  // Use version files if available, otherwise use skill.files
  const currentFiles = useMemo(() => {
    if (hasVersions && versionFiles.length > 0) {
      return versionFiles;
    }
    if (skill?.files && skill.files.length > 0) {
      return skill.files;
    }
    return versionFiles;
  }, [skill?.files, versionFiles, hasVersions]);

  const treeData = useMemo(() => buildFileTree(currentFiles), [currentFiles]);

  const handleSelect = useCallback(
    async (item: TreeDataItem | undefined) => {
      if (!skill || !item) return;

      const file = currentFiles.find((f) => f.path === item.id);
      if (!file || file.is_dir) return;

      setSelectedFile(item.id);
      setLoading(true);

      try {
        // Pass skill object to handle search results not in skills state
        const content = await getFileContent(
          skill.id,
          file.path,
          selectedVersion || undefined,
          skill,
        );
        setFileContent(content || '');
      } catch (error) {
        console.error('Failed to load file content');
      } finally {
        setLoading(false);
      }
    },
    [skill, currentFiles, selectedVersion, getFileContent],
  );

  // Auto-select SKILL.md or README on open
  useEffect(() => {
    if (open && skill && currentFiles.length > 0 && !selectedFile) {
      // Priority: SKILL.md > README.md > index.md
      const priorityFiles = ['skill.md', 'readme.md', 'index.md'];
      let targetFile: SkillFileEntry | undefined;

      for (const priority of priorityFiles) {
        targetFile = currentFiles.find(
          (f) => f.name.toLowerCase() === priority && !f.is_dir,
        );
        if (targetFile) break;
      }

      if (targetFile) {
        handleSelect({ id: targetFile.path } as TreeDataItem);
      }
    }
  }, [open, skill?.id, currentFiles.length]);

  const renderFileContent = () => {
    if (!selectedFile) {
      return (
        <div className="flex flex-col items-center justify-center py-24 text-text-secondary">
          <FileText className="size-12 mb-4 opacity-50" />
          <p>Select a file to view</p>
        </div>
      );
    }

    if (loading) {
      return (
        <div className="flex justify-center py-10">
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

  if (!open || !skill) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-bg-base">
      {/* Page Header with Back Button */}
      <header className="flex items-center justify-between px-5 py-4 border-b border-border bg-bg-base">
        <Button variant="outline" onClick={onClose}>
          <ArrowBigLeft />
          {t('common.back') || 'Back'}
        </Button>
        <div className="flex items-center gap-2">
          {hasVersions ? (
            <Select
              value={selectedVersion}
              onValueChange={setSelectedVersion}
              disabled={versionLoading}
            >
              <SelectTrigger className="w-[120px] h-8 text-xs">
                <Tag className="size-3 mr-1" />
                <SelectValue placeholder="Version" />
              </SelectTrigger>
              <SelectContent>
                {availableVersions.map((version) => (
                  <SelectItem key={version} value={version}>
                    v{version}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : (
            skill.metadata?.version && (
              <Badge variant="outline" className="text-xs">
                <Tag className="size-3 mr-1" />v{skill.metadata.version}
              </Badge>
            )
          )}
        </div>
      </header>

      {/* Main Content Area */}
      <div className="flex flex-1 overflow-hidden bg-bg-base">
        {/* Sidebar - File Tree */}
        <div className="w-80 border-r border-border flex flex-col bg-bg-base">
          <div className="p-4 border-b border-border bg-bg-base">
            <h2 className="font-semibold text-lg truncate">{skill.name}</h2>
            {skill.metadata?.description && (
              <p className="text-text-secondary text-xs mt-2">
                {skill.metadata.description}
              </p>
            )}
            <div className="flex flex-wrap gap-1 mt-2">
              {skill.metadata?.tags?.map((tag) => (
                <Badge key={tag} variant="secondary">
                  {tag}
                </Badge>
              ))}
            </div>
          </div>

          <div className="flex-1 overflow-auto p-2">
            {/* File Tree */}
            {versionLoading ? (
              <div className="flex justify-center py-10">
                <Spin size="default" />
              </div>
            ) : currentFiles.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-10 text-text-secondary">
                <FolderOpen className="size-8 mb-2 opacity-50" />
                <p className="text-sm">
                  {skill?.source_type === 'search' && !(skill as any)._folderId
                    ? 'Please reindex skills in settings to view files'
                    : t('skills.noFiles') || 'No files'}
                </p>
              </div>
            ) : (
              <div>
                <p className="text-text-secondary text-xs pl-2 mb-2">
                  {t('skills.files') || 'Files'}
                  {currentFiles.length > 0 && (
                    <span className="ml-1 text-text-tertiary">
                      ({currentFiles.filter((f) => !f.is_dir).length} files)
                    </span>
                  )}
                </p>
                <TreeView
                  data={treeData}
                  initialSelectedItemId={selectedFile || undefined}
                  onSelectChange={handleSelect}
                  expandAll
                  defaultNodeIcon={FolderOpen}
                  defaultLeafIcon={FileText}
                />
              </div>
            )}
          </div>
        </div>

        {/* Main Content */}
        <div className="flex-1 overflow-auto p-6 bg-bg-base">
          {renderFileContent()}
        </div>
      </div>
    </div>
  );
};

export default memo(SkillDetail);
