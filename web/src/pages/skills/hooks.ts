import fileManagerService from '@/services/file-manager-service';
import { getAuthorization } from '@/utils/authorization-util';
import { message } from 'antd';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Skill, SkillFileEntry, SkillMetadata } from './types';
import {
  filterUploadFiles,
  parseFrontmatter,
  validateSkillFormat as validateSkillFormatImpl,
} from './validation';

const SKILLS_FOLDER = 'skills';

// Helper to get file extension
const getFileExt = (filename: string): string => {
  const parts = filename.split('.');
  return parts.length > 1 ? parts.pop()!.toLowerCase() : '';
};

// Helper to check if file is markdown
export const isMarkdownFile = (filename: string): boolean => {
  const mdExts = ['md', 'markdown', 'mdown', 'mkd'];
  return mdExts.includes(getFileExt(filename));
};

// Helper to parse YAML-like metadata from markdown frontmatter
export const parseMetadata = (
  content: string,
): { metadata: SkillMetadata; body: string } => {
  const { metadata, body } = parseFrontmatter(content);
  return { metadata, body };
};

// Export validation function from validation module
export { validateSkillFormatImpl as validateSkillFormat };

// Re-export validation utilities for use in components
export {
  isMacJunkPath,
  isTextFile,
  parseFrontmatter,
  sanitizeRelPath,
} from './validation';

// Hook to manage skills
export const useSkills = () => {
  const { t } = useTranslation();
  const [skills, setSkills] = useState<Skill[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');

  // Fetch file content
  const fetchFileContent = async (fileId: string): Promise<string | null> => {
    try {
      const response = await fileManagerService.getFile({}, fileId);
      // Response is blob, need to convert to text
      const blob = response.data as Blob;

      // Use FileReader for better browser compatibility
      return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(reader.result as string);
        reader.onerror = () => reject(reader.error);
        reader.readAsText(blob);
      });
    } catch (error) {
      console.error('Error fetching file content:', error);
      return null;
    }
  };

  // Fetch details of a specific skill (with version support)
  const fetchSkillDetails = async (
    folderId: string,
    folderName: string,
  ): Promise<Skill | null> => {
    try {
      // First, list the skill folder to find version folders
      const { data: skillFolderData } = await fileManagerService.listFile({
        parent_id: folderId,
      });

      if (skillFolderData.code !== 0) return null;

      const skillItems = skillFolderData.data?.files || [];

      // Find version folders (folders that match semver pattern like x.y.z)
      const versionFolders = skillItems.filter(
        (f: any) => f.type === 'folder' && /^\d+\.\d+\.\d+/.test(f.name),
      );

      if (versionFolders.length === 0) {
        // No version folders found - fallback to legacy structure
        return fetchSkillDetailsLegacy(folderId, folderName, skillItems);
      }

      // Sort versions by version number (descending)
      const sortedVersions = versionFolders.sort((a: any, b: any) => {
        const va = a.name.split('.').map(Number);
        const vb = b.name.split('.').map(Number);
        for (let i = 0; i < Math.max(va.length, vb.length); i++) {
          const na = va[i] || 0;
          const nb = vb[i] || 0;
          if (na !== nb) return nb - na; // Descending order
        }
        return 0;
      });

      const allVersions = sortedVersions.map((v: any) => v.name);
      const latestVersionFolder = sortedVersions[0];
      const versionFolderId = latestVersionFolder.id;
      const versionName = latestVersionFolder.name;

      // Get all files recursively in the latest version folder
      const fileEntries: SkillFileEntry[] = [];
      let readmeContent: string | null = null;
      let firstFileDate: string | null = null;

      // Recursively fetch all files
      const fetchFilesRecursive = async (
        parentId: string,
        basePath: string = '',
      ) => {
        const { data } = await fileManagerService.listFile({
          parent_id: parentId,
        });
        if (data.code !== 0) return;

        const files = data.data?.files || [];

        // Track date from first encountered file
        if (!firstFileDate && files.length > 0) {
          firstFileDate = files[0]?.create_date || files[0]?.update_date;
        }

        for (const f of files) {
          const path = basePath ? `${basePath}/${f.name}` : f.name;

          fileEntries.push({
            name: f.name,
            path: path,
            is_dir: f.type === 'folder',
            size: f.size || 0,
          });

          // Check for SKILL.md first, then README.md for metadata
          const lowerName = f.name.toLowerCase();
          if (
            lowerName === 'skill.md' ||
            lowerName === 'readme.md' ||
            lowerName === 'index.md'
          ) {
            if (!readmeContent) {
              readmeContent = await fetchFileContent(f.id);
            }
          }

          // Recursively fetch subfolder contents
          if (f.type === 'folder') {
            await fetchFilesRecursive(f.id, path);
          }
        }
      };

      await fetchFilesRecursive(versionFolderId);

      // Parse metadata from README
      let metadata: SkillMetadata = {};
      let description = '';

      if (readmeContent) {
        const parsed = parseMetadata(readmeContent);
        metadata = parsed.metadata;
        description = metadata.description || parsed.body.slice(0, 200);
      }

      // Get dates
      const createDate = firstFileDate || new Date().toISOString();
      const updateDate = createDate;

      return {
        id: folderId,
        name: metadata.name || folderName,
        description,
        source_type: 'local',
        created_at: new Date(createDate).getTime(),
        updated_at: new Date(updateDate).getTime(),
        files: fileEntries,
        metadata: { ...metadata, version: versionName },
        versions: allVersions,
      };
    } catch (error) {
      console.error('Error fetching skill details:', error);
      return null;
    }
  };

  // Legacy fetch for skills without version structure
  const fetchSkillDetailsLegacy = async (
    folderId: string,
    folderName: string,
    skillItems: any[],
  ): Promise<Skill | null> => {
    try {
      const fileEntries: SkillFileEntry[] = [];
      let readmeContent: string | null = null;
      let firstFileDate: string | null = null;

      // Recursively fetch all files
      const fetchFilesRecursive = async (
        parentId: string,
        basePath: string = '',
      ) => {
        const { data } = await fileManagerService.listFile({
          parent_id: parentId,
        });
        if (data.code !== 0) return;

        const files = data.data?.files || [];

        if (!firstFileDate && files.length > 0) {
          firstFileDate = files[0]?.create_date || files[0]?.update_date;
        }

        for (const f of files) {
          const path = basePath ? `${basePath}/${f.name}` : f.name;

          fileEntries.push({
            name: f.name,
            path: path,
            is_dir: f.type === 'folder',
            size: f.size || 0,
          });

          const lowerName = f.name.toLowerCase();
          if (
            lowerName === 'skill.md' ||
            lowerName === 'readme.md' ||
            lowerName === 'index.md'
          ) {
            if (!readmeContent) {
              readmeContent = await fetchFileContent(f.id);
            }
          }

          if (f.type === 'folder') {
            await fetchFilesRecursive(f.id, path);
          }
        }
      };

      // Process items from the skill folder
      for (const f of skillItems) {
        if (f.type === 'folder') {
          await fetchFilesRecursive(f.id, f.name);
        } else {
          fileEntries.push({
            name: f.name,
            path: f.name,
            is_dir: false,
            size: f.size || 0,
          });

          const lowerName = f.name.toLowerCase();
          if (
            lowerName === 'skill.md' ||
            lowerName === 'readme.md' ||
            lowerName === 'index.md'
          ) {
            if (!readmeContent) {
              readmeContent = await fetchFileContent(f.id);
            }
          }
        }
      }

      let metadata: SkillMetadata = {};
      let description = '';

      if (readmeContent) {
        const parsed = parseMetadata(readmeContent);
        metadata = parsed.metadata;
        description = metadata.description || parsed.body.slice(0, 200);
      }

      const createDate = firstFileDate || new Date().toISOString();

      return {
        id: folderId,
        name: metadata.name || folderName,
        description,
        source_type: 'local',
        created_at: new Date(createDate).getTime(),
        updated_at: new Date(createDate).getTime(),
        files: fileEntries,
        metadata,
      };
    } catch (error) {
      console.error('Error fetching legacy skill details:', error);
      return null;
    }
  };

  // Ensure skills folder exists, returns folder ID
  const ensureSkillsFolder = async (): Promise<string | null> => {
    try {
      // List root files to find skills folder
      const { data } = await fileManagerService.listFile({});

      if (data.code !== 0) return null;

      const rootId = data.data?.parent_folder?.id;
      const files = data.data?.files || [];

      // Check if skills folder exists
      const skillsFolder = files.find(
        (f: any) => f.name === SKILLS_FOLDER && f.type === 'folder',
      );

      if (skillsFolder) {
        return skillsFolder.id;
      }

      // Create skills folder
      const createRes = await fileManagerService.createFolder({
        name: SKILLS_FOLDER,
        type: 'folder',
        parent_id: rootId,
      });

      if (createRes.data.code === 0) {
        return createRes.data.data?.id || null;
      }

      return null;
    } catch (error) {
      console.error('Error ensuring skills folder:', error);
      return null;
    }
  };

  // Fetch skills from file API
  const fetchSkills = useCallback(async () => {
    setLoading(true);
    try {
      // First, ensure skills folder exists and get its ID
      const skillsFolderId = await ensureSkillsFolder();

      if (!skillsFolderId) {
        throw new Error('Skills folder not found');
      }

      // List all skill directories
      const { data } = await fileManagerService.listFile({
        parent_id: skillsFolderId,
      });

      if (data.code !== 0) throw new Error('Failed to fetch skills');

      const skillFolders =
        data.data?.files?.filter((f: any) => f.type === 'folder') || [];

      // Fetch details for each skill
      const skillsData: Skill[] = await Promise.all(
        skillFolders.map(async (folder: any) => {
          const skill = await fetchSkillDetails(folder.id, folder.name);
          return skill;
        }),
      );

      setSkills(skillsData.filter(Boolean));
    } catch (error) {
      console.error('Error fetching skills:', error);
      message.error(t('skills.fetchError'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  // Upload a new skill with proper directory structure (with version support)
  const uploadSkill = useCallback(
    async (name: string, version: string, files: File[]): Promise<boolean> => {
      try {
        setLoading(true);

        // Filter out ignored/junk files first
        const filteredFiles = filterUploadFiles(files);

        // Validate skill format
        const validation = await validateSkillFormatImpl(filteredFiles);
        if (!validation.valid) {
          const errorKey = `skills.validation.${validation.error}`;
          const errorMsg = t(errorKey) || t('skills.validation.invalid');
          message.error(errorMsg);
          return false;
        }

        // Get skills folder ID
        const skillsFolderId = await ensureSkillsFolder();

        if (!skillsFolderId) throw new Error('Skills folder not found');

        const skillNameNormalized = name.replace(/\s+/g, '-').toLowerCase();

        // Check if skill folder exists
        const { data: existingData } = await fileManagerService.listFile({
          parent_id: skillsFolderId,
        });

        let skillFolderId: string;

        if (existingData.code === 0) {
          const existingSkill = existingData.data?.files?.find(
            (f: any) => f.name === skillNameNormalized && f.type === 'folder',
          );

          if (existingSkill) {
            // Skill exists, check if version already exists
            const { data: versionData } = await fileManagerService.listFile({
              parent_id: existingSkill.id,
            });

            if (versionData.code === 0) {
              const existingVersion = versionData.data?.files?.find(
                (f: any) => f.name === version && f.type === 'folder',
              );

              if (existingVersion) {
                message.error(
                  t('skills.versionExists') || 'This version already exists',
                );
                return false;
              }
            }

            skillFolderId = existingSkill.id;
          } else {
            // Create skill folder
            const folderRes = await fileManagerService.createFolder({
              name: skillNameNormalized,
              type: 'folder',
              parent_id: skillsFolderId,
            });

            if (folderRes.data.code !== 0) {
              throw new Error('Failed to create skill folder');
            }

            skillFolderId = folderRes.data.data?.id;
          }
        } else {
          throw new Error('Failed to list skills folder');
        }

        if (!skillFolderId) throw new Error('Failed to get skill folder ID');

        // Create version folder
        const versionRes = await fileManagerService.createFolder({
          name: version,
          type: 'folder',
          parent_id: skillFolderId,
        });

        if (versionRes.data.code !== 0) {
          throw new Error('Failed to create version folder');
        }

        const versionFolderId = versionRes.data.data?.id;

        if (!versionFolderId)
          throw new Error('Failed to get version folder ID');

        // Upload files recursively to preserve directory structure
        const uploadFileWithStructure = async (
          file: File,
          parentId: string,
        ) => {
          const relativePath = (file as any).webkitRelativePath || file.name;
          const pathParts = relativePath.split('/');

          // If file is in root directory (no subdirectories)
          if (pathParts.length === 1) {
            const formData = new FormData();
            formData.append('parent_id', parentId);
            formData.append('file', file);
            await fileManagerService.uploadFile(formData);
            return;
          }

          // Navigate/create directory structure
          let currentParentId = parentId;
          for (let i = 0; i < pathParts.length - 1; i++) {
            const dirName = pathParts[i];

            // List current directory to check if subdirectory exists
            const { data: listData } = await fileManagerService.listFile({
              parent_id: currentParentId,
            });

            if (listData.code !== 0) {
              throw new Error(`Failed to list directory: ${dirName}`);
            }

            const existingDir = listData.data?.files?.find(
              (f: any) => f.name === dirName && f.type === 'folder',
            );

            if (existingDir) {
              currentParentId = existingDir.id;
            } else {
              // Create subdirectory
              const createRes = await fileManagerService.createFolder({
                name: dirName,
                type: 'folder',
                parent_id: currentParentId,
              });

              if (createRes.data.code !== 0) {
                throw new Error(`Failed to create directory: ${dirName}`);
              }

              currentParentId = createRes.data.data?.id;
            }
          }

          // Upload file to the final directory
          const formData = new FormData();
          formData.append('parent_id', currentParentId);
          formData.append('file', file);
          await fileManagerService.uploadFile(formData);
        };

        // Upload all files sequentially to avoid race conditions
        for (const file of filteredFiles) {
          await uploadFileWithStructure(file, versionFolderId);
        }

        message.success(t('skills.uploadSuccess'));
        await fetchSkills();
        return true;
      } catch (error) {
        console.error('Error uploading skill:', error);
        message.error(t('skills.uploadError'));
        return false;
      } finally {
        setLoading(false);
      }
    },
    [t, fetchSkills],
  );

  // Delete a skill
  const deleteSkill = useCallback(
    async (skillId: string): Promise<boolean> => {
      try {
        const { data } = await fileManagerService.removeFile({
          ids: [skillId],
        });

        if (data.code !== 0) throw new Error('Failed to delete skill');

        message.success(t('skills.deleteSuccess'));
        await fetchSkills();
        return true;
      } catch (error) {
        console.error('Error deleting skill:', error);
        message.error(t('skills.deleteError'));
        return false;
      }
    },
    [t, fetchSkills],
  );

  // Recursively find file by path in folder structure
  // For versioned skills, automatically finds the version folder first
  const findFileByPath = async (
    folderId: string,
    targetPath: string,
    version?: string,
  ): Promise<any | null> => {
    let currentFolderId = folderId;

    // If version is provided, first find the version folder
    if (version) {
      const { data } = await fileManagerService.listFile({
        parent_id: currentFolderId,
      });
      if (data.code !== 0) return null;

      const files = data.data?.files || [];
      const versionFolder = files.find(
        (f: any) => f.name === version && f.type === 'folder',
      );

      if (!versionFolder) return null;
      currentFolderId = versionFolder.id;
    }

    // Now find the file in the version folder (or original folder if no version)
    const parts = targetPath.split('/');

    for (let i = 0; i < parts.length; i++) {
      const { data } = await fileManagerService.listFile({
        parent_id: currentFolderId,
      });
      if (data.code !== 0) return null;

      const files = data.data?.files || [];
      const part = parts[i];

      // Check if this is the last part (the file)
      if (i === parts.length - 1) {
        const file = files.find((f: any) => f.name === part);
        return file || null;
      }

      // This is a folder, find it and continue
      const subFolder = files.find(
        (f: any) => f.name === part && f.type === 'folder',
      );
      if (!subFolder) return null;
      currentFolderId = subFolder.id;
    }

    return null;
  };

  // Get file content for a skill
  // Automatically handles versioned skills by checking skill.metadata.version
  const getSkillFileContent = useCallback(
    async (
      skillId: string,
      filePath: string,
      version?: string,
    ): Promise<string | null> => {
      try {
        // If version is not provided, try to find it from the skill
        let targetVersion = version;
        if (!targetVersion) {
          const skill = skills.find((s) => s.id === skillId);
          targetVersion = skill?.metadata?.version;
        }

        // Handle both file name and file path
        const file = await findFileByPath(skillId, filePath, targetVersion);
        if (!file) return null;
        return await fetchFileContent(file.id);
      } catch (error) {
        console.error('Error getting skill file content:', error);
        return null;
      }
    },
    [skills],
  );

  // Fetch files for a specific version of a skill
  const getSkillVersionFiles = useCallback(
    async (skillId: string, version: string): Promise<SkillFileEntry[]> => {
      try {
        // First, list the skill folder to find the version folder
        const { data: skillFolderData } = await fileManagerService.listFile({
          parent_id: skillId,
        });

        if (skillFolderData.code !== 0) return [];

        const skillItems = skillFolderData.data?.files || [];
        const versionFolder = skillItems.find(
          (f: any) => f.name === version && f.type === 'folder',
        );

        if (!versionFolder) return [];

        const fileEntries: SkillFileEntry[] = [];

        // Recursively fetch all files in the version folder
        const fetchFilesRecursive = async (
          parentId: string,
          basePath: string = '',
        ) => {
          const { data } = await fileManagerService.listFile({
            parent_id: parentId,
          });
          if (data.code !== 0) return;

          const files = data.data?.files || [];

          for (const f of files) {
            const path = basePath ? `${basePath}/${f.name}` : f.name;

            fileEntries.push({
              name: f.name,
              path: path,
              is_dir: f.type === 'folder',
              size: f.size || 0,
            });

            if (f.type === 'folder') {
              await fetchFilesRecursive(f.id, path);
            }
          }
        };

        await fetchFilesRecursive(versionFolder.id);
        return fileEntries;
      } catch (error) {
        console.error('Error fetching skill version files:', error);
        return [];
      }
    },
    [],
  );

  // Filter skills by search query
  const filteredSkills = skills.filter(
    (skill) =>
      skill.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      skill.description?.toLowerCase().includes(searchQuery.toLowerCase()) ||
      skill.metadata?.tags?.some((tag) =>
        tag.toLowerCase().includes(searchQuery.toLowerCase()),
      ),
  );

  useEffect(() => {
    fetchSkills();
  }, [fetchSkills]);

  return {
    skills,
    filteredSkills,
    loading,
    searchQuery,
    setSearchQuery,
    fetchSkills,
    uploadSkill,
    deleteSkill,
    getSkillFileContent,
    getSkillVersionFiles,
  };
};

// Skill Search Config Hook
interface FieldWeight {
  enabled: boolean;
  weight: number;
}

interface FieldConfig {
  name: FieldWeight;
  tags: FieldWeight;
  description: FieldWeight;
  content: FieldWeight;
}

interface SkillSearchConfig {
  id?: string;
  tenant_id?: string;
  embd_id: string;
  vector_similarity_weight: number;
  similarity_threshold: number;
  field_config: FieldConfig;
  rerank_id?: string;
  top_k: number;
}

export const useSkillSearchConfig = () => {
  const { t } = useTranslation();
  const [config, setConfig] = useState<SkillSearchConfig | null>(null);
  const [loading, setLoading] = useState(false);

  // Fetch config
  const fetchConfig = useCallback(async (embdId?: string) => {
    try {
      const response = await fetch(
        `/api/v1/skill/search/config?embd_id=${embdId || ''}`,
        {
          headers: {
            Authorization: getAuthorization(),
          },
        },
      );
      const data = await response.json();
      if (data.code === 0 && data.data) {
        setConfig(data.data);
        return data.data;
      }
      return null;
    } catch (error) {
      console.error('Error fetching skill search config:', error);
      return null;
    }
  }, []);

  // Save config
  const saveConfig = useCallback(
    async (configData: SkillSearchConfig): Promise<boolean> => {
      try {
        setLoading(true);
        const response = await fetch('/api/v1/skill/search/config', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Authorization: getAuthorization(),
          },
          body: JSON.stringify(configData),
        });
        const data = await response.json();
        if (data.code === 0) {
          setConfig(data.data);
          message.success(t('skillSearch.saveSuccess'));
          return true;
        }
        message.error(data.message || t('skillSearch.saveError'));
        return false;
      } catch (error) {
        console.error('Error saving skill search config:', error);
        message.error(t('skillSearch.saveError'));
        return false;
      } finally {
        setLoading(false);
      }
    },
    [t],
  );

  // Reindex all skills
  const reindex = useCallback(async (): Promise<boolean> => {
    try {
      setLoading(true);
      const response = await fetch('/api/v1/skill/search/reindex', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: getAuthorization(),
        },
        body: JSON.stringify({}),
      });
      const data = await response.json();
      if (data.code === 0) {
        message.success(t('skillSearch.reindexSuccess'));
        return true;
      }
      message.error(data.message || t('skillSearch.reindexError'));
      return false;
    } catch (error) {
      console.error('Error reindexing skills:', error);
      message.error(t('skillSearch.reindexError'));
      return false;
    } finally {
      setLoading(false);
    }
  }, [t]);

  // Initialize index
  const initializeIndex = useCallback(
    async (embdId: string): Promise<boolean> => {
      try {
        const response = await fetch(
          `/api/v1/skill/search/init?embd_id=${embdId}`,
          {
            method: 'POST',
            headers: {
              Authorization: getAuthorization(),
            },
          },
        );
        const data = await response.json();
        return data.code === 0;
      } catch (error) {
        console.error('Error initializing skill search index:', error);
        return false;
      }
    },
    [],
  );

  // Search skills
  const searchSkills = useCallback(
    async (query: string, page = 1, pageSize = 10) => {
      try {
        const response = await fetch('/api/v1/skill/search', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Authorization: getAuthorization(),
          },
          body: JSON.stringify({
            query,
            page,
            page_size: pageSize,
          }),
        });
        const data = await response.json();
        if (data.code === 0 && data.data) {
          // Transform backend results to Skill[] format
          const skills: Skill[] = (data.data.skills || []).map(
            (result: any) => ({
              id: result.skill_id,
              name: result.name,
              description: result.description,
              source_type: 'search',
              created_at: Date.now(),
              updated_at: Date.now(),
              metadata: {
                tags: result.tags || [],
                score: result.score,
                bm25_score: result.bm25_score,
                vector_score: result.vector_score,
              },
              files: [],
            }),
          );
          return {
            skills,
            total: data.data.total || 0,
          };
        }
        return { skills: [], total: 0 };
      } catch (error) {
        console.error('Error searching skills:', error);
        return { skills: [], total: 0 };
      }
    },
    [],
  );

  // Get index status
  const getIndexStatus = useCallback(async () => {
    try {
      const response = await fetch('/api/v1/skill/search/status', {
        headers: {
          Authorization: getAuthorization(),
        },
      });
      const data = await response.json();
      if (data.code === 0) {
        return data.data;
      }
      return null;
    } catch (error) {
      console.error('Error getting skill index status:', error);
      return null;
    }
  }, []);

  return {
    config,
    configLoading: loading,
    fetchConfig,
    saveConfig,
    reindex,
    initializeIndex,
    searchSkills,
    getIndexStatus,
  };
};
