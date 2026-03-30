import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { Spin } from '@/components/ui/spin';
import { TreeDataItem, TreeView } from '@/components/ui/tree-view';
import { ArrowLeft, FileCode, FileText, FolderOpen, Tag } from 'lucide-react';
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
  ) => Promise<string | null>;
  getVersionFiles?: (
    skillId: string,
    version: string,
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

  // Initialize selected version when skill changes
  useEffect(() => {
    if (skill && hasVersions) {
      const defaultVersion = skill.metadata?.version || availableVersions[0];
      setSelectedVersion(defaultVersion);
    } else {
      setSelectedVersion('');
      setVersionFiles([]);
    }
  }, [skill?.id, skill?.versions]);

  // Load files when version changes
  useEffect(() => {
    const loadVersionFiles = async () => {
      if (!skill || !selectedVersion || !getVersionFiles) {
        setVersionFiles([]);
        return;
      }

      // If it's the default version, use skill.files
      if (selectedVersion === skill.metadata?.version) {
        setVersionFiles(skill.files);
        return;
      }

      setVersionLoading(true);
      try {
        const files = await getVersionFiles(skill.id, selectedVersion);
        setVersionFiles(files);
      } catch (error) {
        console.error('Failed to load version files:', error);
        setVersionFiles([]);
      } finally {
        setVersionLoading(false);
      }
    };

    loadVersionFiles();
  }, [skill?.id, selectedVersion, getVersionFiles]);

  // Reset selected file when version changes
  useEffect(() => {
    setSelectedFile(null);
    setFileContent('');
  }, [selectedVersion]);

  // Use version files if available, otherwise use skill.files
  const currentFiles = useMemo(() => {
    if (hasVersions && versionFiles.length > 0) {
      return versionFiles;
    }
    return skill?.files || [];
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
        const content = await getFileContent(
          skill.id,
          file.path,
          selectedVersion || undefined,
        );
        setFileContent(content || '');
      } catch (error) {
        console.error('Failed to load file content');
      } finally {
        setLoading(false);
      }
    },
    [skill, currentFiles, getFileContent, selectedVersion],
  );

  // Auto-select SKILL.md or README on open
  useEffect(() => {
    if (skill && open && currentFiles.length > 0 && !selectedFile) {
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
  }, [skill?.id, open, currentFiles, handleSelect]);

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

  return (
    <Sheet open={open} onOpenChange={(v) => !v && onClose()}>
      <SheetContent side="right" className="w-[90%] sm:max-w-[90%] p-0">
        {skill && (
          <div className="flex h-full">
            {/* Sidebar - File Tree */}
            <div className="w-80 border-r border-border-secondary flex flex-col bg-bg-elevated">
              <div className="p-4 border-b border-border-secondary bg-bg-elevated">
                <Button variant="ghost" className="mb-2 px-0" onClick={onClose}>
                  <ArrowLeft className="mr-2 size-4" />
                  {t('skills.backToSkills') || 'Back to Skills'}
                </Button>
                <SheetHeader className="p-0 space-y-2">
                  <div className="flex items-center gap-2 flex-wrap">
                    <SheetTitle className="text-left truncate">
                      {skill.name}
                    </SheetTitle>
                    {hasVersions ? (
                      <Select
                        value={selectedVersion}
                        onValueChange={setSelectedVersion}
                        disabled={versionLoading}
                      >
                        <SelectTrigger className="w-[120px] h-7 text-xs">
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
                          <Tag className="size-3 mr-1" />v
                          {skill.metadata.version}
                        </Badge>
                      )
                    )}
                  </div>
                </SheetHeader>
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
            <div className="flex-1 overflow-auto p-6 bg-bg-container">
              {renderFileContent()}
            </div>
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
};

export default memo(SkillDetail);
