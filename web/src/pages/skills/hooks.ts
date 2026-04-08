import fileManagerService from '@/services/file-manager-service';
import skillsHubService from '@/services/skills-hub-service';
import { getAuthorization } from '@/utils/authorization-util';
import { message } from 'antd';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type {
  Skill,
  SkillFileEntry,
  SkillMetadata,
  SkillSearchConfig,
  SkillsHub,
} from './types';
import {
  filterUploadFiles,
  isTextFile,
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

// Normalize timestamp-like values from backend to milliseconds.
// Supports epoch seconds, epoch milliseconds and ISO datetime strings.
const toTimestampMs = (value: unknown): number | null => {
  if (value === null || value === undefined || value === '') return null;

  const normalizeEpoch = (raw: number): number | null => {
    if (!Number.isFinite(raw)) return null;

    let n = raw;
    // Convert unit by magnitude: ns -> us -> ms -> s.
    // Current epoch in ms is around 1e12.
    if (n > 1e17)
      n = n / 1e6; // nanoseconds
    else if (n > 1e14)
      n = n / 1e3; // microseconds
    else if (n < 1e11) n = n * 1e3; // seconds

    return Math.round(n);
  };

  if (typeof value === 'number' && Number.isFinite(value)) {
    return normalizeEpoch(value);
  }

  if (typeof value === 'string') {
    const trimmed = value.trim();
    if (!trimmed) return null;

    const numeric = Number(trimmed);
    if (!Number.isNaN(numeric)) {
      return normalizeEpoch(numeric);
    }

    const parsed = Date.parse(trimmed);
    return Number.isNaN(parsed) ? null : parsed;
  }

  return null;
};

const pickSkillTimestamp = (result: any): number => {
  const candidates = [
    result?.updated_at,
    result?.updatedAt,
    result?.update_time,
    result?.updateTime,
    result?.update_date,
    result?.modified_at,
    result?.modifiedAt,
    result?.metadata?.updated_at,
    result?.metadata?.updatedAt,
    result?.metadata?.update_time,
    result?.metadata?.updateTime,
    result?.metadata?.update_date,
    result?.skill?.updated_at,
    result?.skill?.updatedAt,
    result?.skill?.update_time,
    result?.skill?.updateTime,
    result?.skill?.update_date,
    result?.created_at,
    result?.createdAt,
    result?.create_time,
    result?.createTime,
    result?.create_date,
    result?.metadata?.created_at,
    result?.metadata?.createdAt,
    result?.metadata?.create_time,
    result?.metadata?.createTime,
    result?.metadata?.create_date,
    result?.skill?.created_at,
    result?.skill?.createdAt,
    result?.skill?.create_time,
    result?.skill?.createTime,
    result?.skill?.create_date,
  ];

  for (const candidate of candidates) {
    const ts = toTimestampMs(candidate);
    if (ts !== null) return ts;
  }

  return Date.now();
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

      // Priority: folderName (user-specified) > metadata.name (from SKILL.md)
      // This allows users to override the skill name from SKILL.md
      const skillName = folderName || metadata.name || 'unnamed-skill';

      return {
        id: skillName, // Use skill name as ID (consistent with search results)
        name: skillName,
        description,
        source_type: 'local',
        created_at: new Date(createDate).getTime(),
        updated_at: new Date(updateDate).getTime(),
        files: fileEntries,
        metadata: { ...metadata, version: versionName },
        versions: allVersions,
        _folderId: folderId, // Internal use for file operations
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

      // Priority: folderName (user-specified) > metadata.name (from SKILL.md)
      // This allows users to override the skill name from SKILL.md
      const skillName = folderName || metadata.name || 'unnamed-skill';

      return {
        id: skillName, // Use skill name as ID (consistent with search results)
        name: skillName,
        description,
        source_type: 'local',
        created_at: new Date(createDate).getTime(),
        updated_at: new Date(createDate).getTime(),
        files: fileEntries,
        metadata,
        _folderId: folderId, // Internal use for file operations
      };
    } catch (error) {
      console.error('Error fetching legacy skill details:', error);
      return null;
    }
  };

  // Ensure skills folder exists, returns folder ID
  const ensureSkillsFolder = useCallback(async (): Promise<string | null> => {
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
  }, []);

  const fetchHubs = useCallback(async (): Promise<SkillsHub[]> => {
    try {
      const result = await skillsHubService.listHubs();
      return result.hubs.map((hub) => ({
        id: hub.id,
        name: hub.name,
        create_time: hub.create_time,
        folder_id: hub.folder_id,
      }));
    } catch (error) {
      console.error('Error fetching skill hubs:', error);
      return [];
    }
  }, []);

  const ensureSkillsHubFolder = useCallback(
    async (
      hubName: string,
      createIfMissing = false,
    ): Promise<string | null> => {
      const skillsFolderId = await ensureSkillsFolder();
      if (!skillsFolderId) return null;

      const { data } = await fileManagerService.listFile({
        parent_id: skillsFolderId,
      });

      if (data.code !== 0) return null;

      const hubFolder = (data.data?.files || []).find(
        (f: any) => f.name === hubName && f.type === 'folder',
      );
      if (hubFolder) return hubFolder.id;

      if (!createIfMissing) return null;

      const createRes = await fileManagerService.createFolder({
        name: hubName,
        type: 'folder',
        parent_id: skillsFolderId,
      });

      if (createRes.data.code !== 0) return null;
      return createRes.data.data?.id || null;
    },
    [ensureSkillsFolder],
  );

  const createHub = useCallback(
    async (hubName: string): Promise<{ id: string; name: string } | null> => {
      try {
        const hub = await skillsHubService.createHub({ name: hubName });
        message.success(
          t('skills.hubCreated') || 'Skills Hub created successfully',
        );
        return hub;
      } catch (error: any) {
        console.error('Error creating skill hub:', error);
        message.error(error.message || t('skills.fetchError'));
        return null;
      }
    },
    [t],
  );

  // Delete a skills hub
  const deleteHub = useCallback(
    async (hubId: string): Promise<boolean> => {
      try {
        await skillsHubService.deleteHub(hubId);
        message.success(
          t('skills.hubDeleted') || 'Skills Hub deleted successfully',
        );
        return true;
      } catch (error: any) {
        console.error('Error deleting skill hub:', error);
        message.error(error.message || t('skills.fetchError'));
        return false;
      }
    },
    [t],
  );

  // Update a skills hub (rename)
  const updateHub = useCallback(
    async (hubId: string, hubName: string): Promise<boolean> => {
      try {
        await skillsHubService.updateHub(hubId, { name: hubName });
        message.success(
          t('skills.hubUpdated') || 'Skills Hub renamed successfully',
        );
        return true;
      } catch (error: any) {
        console.error('Error updating skill hub:', error);
        message.error(error.message || t('skills.fetchError'));
        return false;
      }
    },
    [t],
  );

  // Fetch skills from file system (fallback when search returns empty)
  const fetchSkillsFromFileSystem = useCallback(
    async (hubName?: string): Promise<{ skills: Skill[]; total: number }> => {
      if (!hubName) {
        return { skills: [], total: 0 };
      }
      try {
        const hubFolderId = await ensureSkillsHubFolder(hubName, false);
        if (!hubFolderId) {
          return { skills: [], total: 0 };
        }

        const { data } = await fileManagerService.listFile({
          parent_id: hubFolderId,
        });

        const skillFolders =
          data.code === 0
            ? data.data?.files?.filter((f: any) => f.type === 'folder') || []
            : [];

        // Fetch details for each skill
        const skillsData: Skill[] = (
          await Promise.all(
            skillFolders.map(async (folder: any) => {
              const skill = await fetchSkillDetails(folder.id, folder.name);
              return skill;
            }),
          )
        ).filter(Boolean);

        return { skills: skillsData, total: skillsData.length };
      } catch (error) {
        console.error('Error fetching skills from file system:', error);
        return { skills: [], total: 0 };
      }
    },
    [ensureSkillsHubFolder],
  );

  // Fetch skills using search API (supports pagination and sorting)
  // Falls back to file system if search returns empty (skills not indexed yet)
  const fetchSkills = useCallback(
    async (
      hubName?: string,
      hubId?: string,
      page = 1,
      pageSize = 50,
      sortBy = 'update_time',
      sortOrder: 'asc' | 'desc' = 'desc',
    ) => {
      if (!hubName || !hubId) {
        setSkills([]);
        return { skills: [], total: 0 };
      }
      setLoading(true);
      try {
        // Use search API with empty query to list all skills
        const response = await fetch('/api/v1/skills/search', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Authorization: getAuthorization(),
          },
          body: JSON.stringify({
            hub_id: hubId,
            query: '', // Empty query = list all
            page,
            page_size: pageSize,
            sort_by: sortBy,
            sort_order: sortOrder,
          }),
        });

        if (!response.ok) {
          throw new Error('Failed to fetch skills');
        }

        const result = await response.json();
        if (result.code !== 0) {
          throw new Error(result.message || 'Failed to fetch skills');
        }

        const searchSkills = result.data?.skills || [];
        const total = result.data?.total || 0;

        // If search returned results, use them
        if (searchSkills.length > 0) {
          const skillsData: Skill[] = searchSkills.map((result: any) => {
            const timestamp = pickSkillTimestamp(result);
            const skillId = result.skill_id || result.name;

            return {
              id: skillId,
              name: result.name,
              description: result.description || '',
              source_type: 'search',
              created_at: timestamp,
              updated_at: timestamp,
              metadata: {
                tags: result.tags || [],
                version: result.version,
              },
              files: [],
              _folderId: result.folder_id,
            };
          });

          setSkills(skillsData);
          return { skills: skillsData, total };
        }

        // Search returned empty, fall back to file system
        console.log(
          '[Skills] Search returned empty, falling back to file system',
        );
        const fsResult = await fetchSkillsFromFileSystem(hubName);
        setSkills(fsResult.skills);
        return fsResult;
      } catch (error) {
        console.error('Error fetching skills:', error);
        // Fall back to file system on error
        const fsResult = await fetchSkillsFromFileSystem(hubName);
        setSkills(fsResult.skills);
        return fsResult;
      } finally {
        setLoading(false);
      }
    },
    [t, fetchSkillsFromFileSystem],
  );

  // Upload a new skill with proper directory structure (with version support)
  const uploadSkill = useCallback(
    async (
      name: string,
      version: string,
      files: File[],
      hubName?: string,
      hubId?: string,
      embdId?: string,
    ): Promise<boolean> => {
      try {
        setLoading(true);
        if (!hubName) throw new Error('Hub name is required');

        // Use hubName for file system operations, hubId for indexing
        const normalizedHubName = hubName.trim();
        const normalizedHubId = hubId?.trim() || normalizedHubName;

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

        // Get hub folder ID (using hub name for file system)
        const hubFolderId = await ensureSkillsHubFolder(
          normalizedHubName,
          true,
        );

        if (!hubFolderId) throw new Error('Skills hub not found');

        const skillNameNormalized = name.replace(/\s+/g, '-').toLowerCase();

        // Check if skill folder exists
        const { data: existingData } = await fileManagerService.listFile({
          parent_id: hubFolderId,
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
              parent_id: hubFolderId,
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

        // Build search index for the uploaded skill
        try {
          // Read all text files and build content
          let skillMetadata: SkillMetadata = {};
          let skillDescription = '';
          const fileContents: { path: string; content: string }[] = [];

          for (const file of filteredFiles) {
            const relativePath = (file as any).webkitRelativePath || file.name;
            if (!isTextFile(relativePath, file.type)) {
              continue;
            }

            const content = await file.text();
            fileContents.push({ path: relativePath, content });

            // Parse metadata from skill.md/readme.md/index.md
            const lowerName = file.name.toLowerCase();
            if (
              lowerName === 'skill.md' ||
              lowerName === 'readme.md' ||
              lowerName === 'index.md'
            ) {
              const parsed = parseMetadata(content);
              skillMetadata = parsed.metadata;
              skillDescription =
                skillMetadata.description || parsed.body.slice(0, 200);
            }
          }

          // Build concatenated content for indexing
          const concatenatedContent = fileContents
            .map((f) => `${f.path}\n===\n${f.content}`)
            .join('\n\n');

          // Index the skill with embd_id from config (if available)
          // Use user-specified name (skillNameNormalized) as skill ID and name
          // This ensures consistency between folder name, skill ID, and display name
          const indexResponse = await fetch('/api/v1/skills/index', {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              Authorization: getAuthorization(),
            },
            body: JSON.stringify({
              hub_id: normalizedHubId,
              embd_id: embdId,
              skills: [
                {
                  id: skillNameNormalized,
                  folder_id: skillFolderId,
                  name: skillNameNormalized,
                  description: skillDescription,
                  tags: skillMetadata.tags || [],
                  content: concatenatedContent,
                },
              ],
            }),
          });

          if (!indexResponse.ok) {
            console.warn(
              '[Skill Index] Failed to index skill:',
              await indexResponse.text(),
            );
          }
        } catch (indexError) {
          // Indexing failure should not block upload success
          console.warn('[Skill Index] Error indexing skill:', indexError);
        }

        message.success(t('skills.uploadSuccess'));
        await fetchSkills(normalizedHubName, normalizedHubId);
        return true;
      } catch (error) {
        console.error('Error uploading skill:', error);
        message.error(t('skills.uploadError'));
        return false;
      } finally {
        setLoading(false);
      }
    },
    [t, fetchSkills, ensureSkillsHubFolder],
  );

  // Delete a skill
  const deleteSkill = useCallback(
    async (
      skillId: string,
      _skillName?: string,
      hubId?: string,
      hubName?: string,
      folderId?: string,
    ): Promise<boolean> => {
      try {
        if (!hubId) throw new Error('Hub ID is required');
        if (!hubName) throw new Error('Hub name is required');
        const normalizedHubId = hubId.trim();
        const normalizedHubName = hubName.trim();

        let targetFolderId: string | null = folderId || null;

        // If folderId not provided, try to find the skill in current skills state
        if (!targetFolderId) {
          const skillInState = skills.find((s) => s.id === skillId);
          if (skillInState && (skillInState as any)._folderId) {
            targetFolderId = (skillInState as any)._folderId;
          }
        }

        // Fallback: search in file system if not found
        if (!targetFolderId) {
          const hubFolderId = await ensureSkillsHubFolder(
            normalizedHubName,
            false,
          );
          if (hubFolderId) {
            const { data: listData } = await fileManagerService.listFile({
              parent_id: hubFolderId,
            });

            if (listData.code === 0) {
              const skillFolder = (listData.data?.files || []).find(
                (f: any) => f.type === 'folder' && f.name === skillId,
              );
              if (skillFolder) {
                targetFolderId = skillFolder.id;
              }
            }
          }
        }

        if (!targetFolderId) {
          throw new Error('Skill not found');
        }

        // Get versions by listing the skill folder
        const { data: versionData } = await fileManagerService.listFile({
          parent_id: targetFolderId,
        });

        let versionsToDelete: string[] = ['latest'];
        if (versionData.code === 0) {
          const versionFolders = (versionData.data?.files || []).filter(
            (f: any) => f.type === 'folder' && /^\d+\.\d+\.\d+/.test(f.name),
          );
          if (versionFolders.length > 0) {
            versionsToDelete = versionFolders.map((f: any) => f.name);
          }
        }

        // Delete search index for all versions
        // Backend uses skillName_version as doc_id (replacing '/' with '_')
        // We need to delete each version's index separately
        console.log(
          `[deleteSkill] Starting index deletion for skillId: ${skillId}, hubId: ${normalizedHubId}`,
        );
        console.log(`[deleteSkill] versionsToDelete:`, versionsToDelete);

        for (const version of versionsToDelete) {
          const indexId =
            version === 'latest' ? skillId : `${skillId}/${version}`;
          try {
            console.log(
              `[deleteSkill] Deleting index: ${indexId} for hub: ${normalizedHubId}`,
            );
            await skillsHubService.deleteSkillIndex(indexId, normalizedHubId);
            console.log(`[deleteSkill] Successfully deleted index: ${indexId}`);
          } catch (indexError: any) {
            console.warn(
              `[deleteSkill] Error deleting skill index for ${indexId}:`,
              indexError?.message || indexError,
            );
          }
        }

        // If we couldn't determine versions from filesystem, try common version formats
        if (versionsToDelete.length === 1 && versionsToDelete[0] === 'latest') {
          // Try to delete the skill with version suffixes
          const commonVersions = ['1.0.0', '0.1.0', '0.0.1', 'latest'];
          for (const version of commonVersions) {
            const indexId = `${skillId}/${version}`;
            try {
              console.log(
                `[deleteSkill] Trying to delete index with version: ${indexId}`,
              );
              await skillsHubService.deleteSkillIndex(indexId, normalizedHubId);
              console.log(
                `[deleteSkill] Successfully deleted index: ${indexId}`,
              );
            } catch {
              // Ignore errors for versions that don't exist
            }
          }
        }

        const { data } = await fileManagerService.removeFile({
          ids: [targetFolderId],
        });

        if (data.code !== 0) throw new Error('Failed to delete skill');

        message.success(t('skills.deleteSuccess'));
        // Refresh skills list using hub name and hub id
        await fetchSkills(normalizedHubName, normalizedHubId);
        return true;
      } catch (error) {
        console.error('Error deleting skill:', error);
        message.error(t('skills.deleteError'));
        return false;
      }
    },
    [t, fetchSkills, ensureSkillsHubFolder, ensureSkillsFolder, skills],
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
    } else {
      // No version specified, try to find the latest version folder
      const { data } = await fileManagerService.listFile({
        parent_id: currentFolderId,
      });
      if (data.code !== 0) return null;

      const files = data.data?.files || [];
      const versionFolders = files.filter(
        (f: any) => f.type === 'folder' && /^\d+\.\d+\.\d+/.test(f.name),
      );

      if (versionFolders.length > 0) {
        // Sort by version number (descending) to get the latest
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
        currentFolderId = sortedVersions[0].id;
      }
      // If no version folders found, stay at current level (legacy structure)
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
  // Can be called with an optional skill object (for search results not in skills state)
  const getSkillFileContent = useCallback(
    async (
      skillId: string,
      filePath: string,
      version?: string,
      skillObj?: Skill,
    ): Promise<string | null> => {
      try {
        // Find the skill to get its folder ID
        // Use provided skill object if available (for search results), otherwise look up in skills state
        const skill = skillObj || skills.find((s) => s.id === skillId);
        if (!skill) return null;

        // Use internal _folderId for file operations
        const folderId = (skill as any)._folderId;
        if (!folderId) return null;

        // If version is not provided, try to find it from the skill or auto-discover
        let targetVersion = version;
        if (!targetVersion) {
          targetVersion = skill?.metadata?.version;
        }

        // Handle both file name and file path
        const file = await findFileByPath(folderId, filePath, targetVersion);
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
  // Can be called with an optional skill object (for search results not in skills state)
  const getSkillVersionFiles = useCallback(
    async (
      skillId: string,
      version: string,
      skillObj?: Skill,
    ): Promise<SkillFileEntry[]> => {
      try {
        // Find the skill to get its folder ID
        // Use provided skill object if available (for search results), otherwise look up in skills state
        const skill = skillObj || skills.find((s) => s.id === skillId);
        if (!skill) return [];

        // Use internal _folderId for file operations
        const folderId = (skill as any)._folderId;
        if (!folderId) return [];

        // First, list the skill folder to find the version folder
        const { data: skillFolderData } = await fileManagerService.listFile({
          parent_id: folderId,
        });

        if (skillFolderData.code !== 0) return [];

        const skillItems = skillFolderData.data?.files || [];

        // If version is not provided, find the latest version folder
        let targetVersion = version;
        if (!targetVersion) {
          // Find all version folders (matching semver pattern x.y.z)
          const versionFolders = skillItems.filter(
            (f: any) => f.type === 'folder' && /^\d+\.\d+\.\d+/.test(f.name),
          );
          if (versionFolders.length === 0) return [];

          // Sort by version number (descending) to get the latest
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
          targetVersion = sortedVersions[0].name;
        }

        const versionFolder = skillItems.find(
          (f: any) => f.name === targetVersion && f.type === 'folder',
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
    [skills],
  );

  // Filter skills by search query
  const filteredSkills = useMemo(
    () =>
      skills.filter(
        (skill) =>
          skill.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
          skill.description
            ?.toLowerCase()
            .includes(searchQuery.toLowerCase()) ||
          skill.metadata?.tags?.some((tag) =>
            tag.toLowerCase().includes(searchQuery.toLowerCase()),
          ),
      ),
    [skills, searchQuery],
  );

  // Fetch skills on mount
  useEffect(() => {
    fetchSkills();
  }, [fetchSkills]);

  // Get skill details by folder ID and name (for loading versions)
  const getSkillDetails = useCallback(
    async (folderId: string, folderName: string): Promise<Skill | null> => {
      return await fetchSkillDetails(folderId, folderName);
    },
    [],
  );

  return {
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
  };
};

// Skill Search Config Hook
export const useSkillSearchConfig = (hubId?: string) => {
  const { t } = useTranslation();
  const [config, setConfig] = useState<SkillSearchConfig | null>(null);
  const [loading, setLoading] = useState(false);

  // Fetch config
  const fetchConfig = useCallback(
    async (embdId?: string, currentHubId?: string) => {
      try {
        const targetHubId = currentHubId || hubId;
        if (!targetHubId) return null;
        const data = await skillsHubService.getConfig(targetHubId, embdId);
        // Cast to local SkillSearchConfig type (structures are compatible)
        setConfig(data as any);
        return data as any;
      } catch (error) {
        console.error('Error fetching skill search config:', error);
        return null;
      }
    },
    [hubId],
  );

  // Save config
  const saveConfig = useCallback(
    async (configData: SkillSearchConfig): Promise<boolean> => {
      try {
        setLoading(true);
        if (!hubId) throw new Error('Hub ID is required');
        const data = await skillsHubService.updateConfig({
          ...configData,
          hub_id: hubId,
        });
        // Cast to local SkillSearchConfig type (structures are compatible)
        setConfig(data as any);
        message.success(t('skillSearch.saveSuccess'));
        return true;
      } catch (error: any) {
        console.error('Error saving skill search config:', error);
        message.error(error.message || t('skillSearch.saveError'));
        return false;
      } finally {
        setLoading(false);
      }
    },
    [t, hubId],
  );

  // Reindex all skills
  const reindex = useCallback(
    async (embdId?: string): Promise<boolean> => {
      try {
        setLoading(true);
        if (!hubId) throw new Error('Hub ID is required');
        await skillsHubService.reindex({
          skills: [],
          hub_id: hubId,
          embd_id: embdId,
        });
        message.success(t('skillSearch.reindexSuccess'));
        return true;
      } catch (error: any) {
        console.error('Error reindexing skills:', error);
        message.error(error.message || t('skillSearch.reindexError'));
        return false;
      } finally {
        setLoading(false);
      }
    },
    [t, hubId],
  );

  // Initialize index
  const initializeIndex = useCallback(async (): Promise<boolean> => {
    try {
      if (!hubId) throw new Error('Hub ID is required');
      // Initialize index is now handled automatically when creating index
      // Call index API directly to ensure index exists
      // embd_id will be fetched from skill search config by backend
      await skillsHubService.indexSkills({ skills: [], hub_id: hubId });
      return true;
    } catch (error) {
      console.error('Error initializing skill search index:', error);
      return false;
    }
  }, [hubId]);

  // Search skills
  const searchSkills = useCallback(
    async (query: string, page = 1, pageSize = 10) => {
      try {
        if (!hubId) return { skills: [], total: 0 };
        const data = await skillsHubService.search({
          hub_id: hubId,
          query,
          page,
          page_size: pageSize,
        });
        // Transform backend results to Skill[] format
        // Use folder_id if available (for file operations), otherwise skill_id
        const skills: Skill[] = (data.skills || []).map((result: any) => {
          // Prefer backend timestamp to avoid all cards showing "just now".
          // Fallback to now only when backend doesn't provide time fields.
          const timestamp = pickSkillTimestamp(result);

          // skill_id from backend is now the skill name (without version suffix)
          const skillId = result.skill_id || result.name;

          return {
            id: skillId, // Use skill name as ID (consistent with list view)
            name: result.name,
            description: result.description,
            source_type: 'search',
            created_at: timestamp,
            updated_at: timestamp,
            metadata: {
              tags: result.tags || [],
              score: result.score,
              bm25_score: result.bm25_score,
              vector_score: result.vector_score,
            },
            files: [],
            _folderId: result.folder_id, // Store folder_id for file operations if needed
          };
        });
        return {
          skills,
          total: data.total || 0,
        };
      } catch (error) {
        console.error('Error searching skills:', error);
        return { skills: [], total: 0 };
      }
    },
    [hubId],
  );

  // Get index status
  const getIndexStatus = useCallback(async () => {
    try {
      const response = await fetch('/api/v1/skills/status', {
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
