import fileManagerService from '@/services/file-manager-service';
import { message } from 'antd';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Skill, SkillFileEntry, SkillMetadata } from './types';
import {
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

  // Fetch details of a specific skill
  const fetchSkillDetails = async (
    folderId: string,
    folderName: string,
  ): Promise<Skill | null> => {
    try {
      // Get all files recursively in the skill folder
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

      await fetchFilesRecursive(folderId);

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
        metadata,
      };
    } catch (error) {
      console.error('Error fetching skill details:', error);
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

  // Upload a new skill
  const uploadSkill = useCallback(
    async (name: string, files: File[]): Promise<boolean> => {
      try {
        setLoading(true);

        // Validate skill format first
        const validation = await validateSkillFormatImpl(files);
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

        // Check if skill with same name already exists
        const { data: existingData } = await fileManagerService.listFile({
          parent_id: skillsFolderId,
        });

        if (existingData.code === 0) {
          const existingSkill = existingData.data?.files?.find(
            (f: any) => f.name === skillNameNormalized && f.type === 'folder',
          );

          if (existingSkill) {
            message.error(t('skills.skillExists'));
            return false;
          }
        }

        // Create skill folder
        const folderRes = await fileManagerService.createFolder({
          name: skillNameNormalized,
          type: 'folder',
          parent_id: skillsFolderId,
        });

        if (folderRes.data.code !== 0) {
          throw new Error('Failed to create skill folder');
        }

        const skillFolderId = folderRes.data.data?.id;

        if (!skillFolderId) throw new Error('Failed to get skill folder ID');

        // Upload files using FormData with path for directory structure
        const formData = new FormData();
        formData.append('parent_id', skillFolderId);

        files.forEach((file: any) => {
          formData.append('file', file);
          formData.append('path', file.webkitRelativePath || file.name);
        });

        const uploadRes = await fileManagerService.uploadFile(formData);

        if (uploadRes.data.code !== 0) {
          throw new Error('Failed to upload files');
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
  const findFileByPath = async (
    folderId: string,
    targetPath: string,
  ): Promise<any | null> => {
    const parts = targetPath.split('/');
    let currentFolderId = folderId;

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
  const getSkillFileContent = useCallback(
    async (skillId: string, filePath: string): Promise<string | null> => {
      try {
        // Handle both file name and file path
        const file = await findFileByPath(skillId, filePath);
        if (!file) return null;
        return await fetchFileContent(file.id);
      } catch (error) {
        console.error('Error getting skill file content:', error);
        return null;
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
  };
};
