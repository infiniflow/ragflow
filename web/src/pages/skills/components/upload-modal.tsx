import { FileUploader } from '@/components/file-uploader';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Modal } from '@/components/ui/modal/modal';
import { Progress } from '@/components/ui/progress';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { CheckCircle, FolderOpen, Globe, Loader2, XCircle } from 'lucide-react';
import React, { useCallback, useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { validateSkillFormat } from '../hooks';
import type { ValidationError } from '../types';
import { findJunkFiles } from '../validation';

interface UploadModalProps {
  open: boolean;
  onCancel: () => void;
  onUpload: (name: string, version: string, files: File[]) => Promise<boolean>;
  loading?: boolean;
}

type GitPlatform = 'github' | 'gitee';

interface GitFile {
  path: string;
  download_url: string;
  type: 'file' | 'dir';
  size: number;
}

const PLATFORM_CONFIG: Record<
  GitPlatform,
  { name: string; apiBase: string; rawBase: string; defaultBranch: string }
> = {
  github: {
    name: 'GitHub',
    apiBase: 'https://api.github.com',
    rawBase: 'https://raw.githubusercontent.com',
    defaultBranch: 'main',
  },
  gitee: {
    name: 'Gitee',
    apiBase: 'https://gitee.com/api/v5',
    rawBase: 'https://gitee.com',
    defaultBranch: 'master',
  },
};

interface UploadFormData {
  name: string;
  version: string;
}

const UploadModal: React.FC<UploadModalProps> = ({
  open,
  onCancel,
  onUpload,
}) => {
  const { t } = useTranslation();
  const [activeTab, setActiveTab] = useState('upload');

  // Upload form with react-hook-form
  const form = useForm<UploadFormData>({
    defaultValues: {
      name: '',
      version: '',
    },
  });
  const { register, handleSubmit, setValue, watch, reset } = form;
  const nameValue = watch('name');

  const [files, setFiles] = useState<File[]>([]);
  const [uploading, setUploading] = useState(false);
  const [progress, setProgress] = useState(0);
  const [validationStatus, setValidationStatus] = useState<
    'valid' | 'invalid' | 'pending' | null
  >(null);
  const [validationMessage, setValidationMessage] = useState<string>('');
  const [, setValidationErrors] = useState<ValidationError[]>([]);
  const [parsedMetadata, setParsedMetadata] = useState<{
    name?: string;
    description?: string;
  } | null>(null);

  // Git import tab state
  const [gitPlatform, setGitPlatform] = useState<GitPlatform>('github');
  const [repoUrl, setRepoUrl] = useState('');
  const [gitVersion, setGitVersion] = useState('');
  const [gitToken, setGitToken] = useState('');
  const [gitImporting, setGitImporting] = useState(false);
  const [gitProgress, setGitProgress] = useState('');
  const [gitValidationStatus, setGitValidationStatus] = useState<
    'valid' | 'invalid' | 'pending' | null
  >(null);
  const [gitValidationMessage, setGitValidationMessage] = useState<string>('');

  const validateName = (value: string): boolean | string => {
    if (!value) {
      return t('skills.skillNameHelp');
    }
    if (!/^[a-zA-Z0-9_-]+$/.test(value)) {
      return t('skills.skillNameHelp');
    }
    return true;
  };

  const validateVersion = (value: string): boolean | string => {
    if (!value) {
      return t('skills.versionRequired') || 'Version is required';
    }
    // Semantic versioning format: x.y.z
    if (!/^\d+\.\d+\.\d+/.test(value)) {
      return (
        t('skills.versionFormatHelp') ||
        'Version must be in semver format (e.g., 1.0.0)'
      );
    }
    return true;
  };

  const validateGitVersion = (value: string): boolean => {
    if (!value) {
      return false;
    }
    return /^\d+\.\d+\.\d+/.test(value);
  };

  const handleOk = handleSubmit(async (data) => {
    if (files.length === 0) {
      return;
    }

    setUploading(true);
    setProgress(0);

    try {
      const success = await onUpload(data.name, data.version, files);

      if (success) {
        reset({ name: '', version: '' });
        setFiles([]);
        onCancel();
      }
    } catch (error) {
      console.error('Upload error:', error);
    } finally {
      setUploading(false);
      setProgress(0);
    }
  });

  const handleCancel = useCallback(() => {
    if (!uploading && !gitImporting) {
      // Reset upload tab state
      reset({ name: '', version: '' });
      setFiles([]);
      setValidationStatus(null);
      setValidationMessage('');
      setValidationErrors([]);
      setParsedMetadata(null);
      // Reset git import tab state
      setActiveTab('upload');
      setRepoUrl('');
      setGitVersion('');
      setGitToken('');
      setGitValidationStatus(null);
      setGitValidationMessage('');
      setGitProgress('');
      onCancel();
    }
  }, [uploading, gitImporting, onCancel, reset]);

  // Handle files change from FileUploader
  const handleFilesChange = useCallback(
    (newFiles: File[]) => {
      setFiles(newFiles);

      // Auto-fill name from folder name if empty
      if (newFiles.length > 0 && !nameValue) {
        const firstFile = newFiles[0];
        const path = (firstFile as any).webkitRelativePath || firstFile.name;
        const folderName = path.split('/')[0];
        if (folderName) {
          setValue('name', folderName, { shouldValidate: true });
        }
      }
    },
    [nameValue, setValue],
  );

  // Validate files when files change
  useEffect(() => {
    const validateFilesAsync = async () => {
      if (files.length === 0) {
        setValidationStatus(null);
        setValidationMessage('');
        setValidationErrors([]);
        setParsedMetadata(null);
        return;
      }

      setValidationStatus('pending');

      try {
        // Check for junk files first
        const junkFiles = findJunkFiles(files);
        if (junkFiles.length > 0) {
          setValidationStatus('invalid');
          const fileNames = junkFiles.slice(0, 3).join(', ');
          const more =
            junkFiles.length > 3 ? ` (+${junkFiles.length - 3} more)` : '';
          setValidationMessage(
            `${t('skills.validation.junkFilesFound') || 'Please remove temporary files before uploading'}: ${fileNames}${more}`,
          );
          setValidationErrors([]);
          setParsedMetadata(null);
          return;
        }

        const result = await validateSkillFormat(files);

        if (result.valid) {
          setValidationStatus('valid');
          setValidationMessage(
            t('skills.validation.valid') || 'Valid skill format',
          );
          setValidationErrors([]);
          setParsedMetadata({
            name: result.name,
            description: result.description,
          });
          // Auto-fill name if extracted from SKILL.md
          if (result.name && !nameValue) {
            setValue('name', result.name, { shouldValidate: true });
          }
        } else {
          setValidationStatus('invalid');
          setParsedMetadata(null);

          // Build detailed error message
          let errorMsg = '';
          if (result.details) {
            errorMsg = `${t(`skills.validation.${result.error}`) || t('skills.validation.invalid')}: ${result.details}`;
          } else {
            errorMsg =
              t(`skills.validation.${result.error}`) ||
              t('skills.validation.invalid');
          }
          setValidationMessage(errorMsg);
        }
      } catch (err) {
        console.error('Validation error:', err);
        setValidationStatus('invalid');
        const errorMsg = err instanceof Error ? err.message : String(err);
        setValidationMessage(
          `${t('skills.validation.error') || 'Validation failed'}: ${errorMsg}`,
        );
        setValidationErrors([]);
        setParsedMetadata(null);
      }
    };

    validateFilesAsync();
  }, [files, t, nameValue, setValue]);

  const isUploadDisabled = validationStatus === 'invalid' || files.length === 0;

  // ===== Git Import Functions =====

  // Parse Git repository URL
  const parseGitUrl = useCallback((url: string, platform: GitPlatform) => {
    const config = PLATFORM_CONFIG[platform];

    if (platform === 'github') {
      // GitHub URL patterns:
      // https://github.com/owner/repo
      // https://github.com/owner/repo/tree/branch/path
      // https://github.com/owner/repo/blob/branch/path/file
      const patterns = [
        /github\.com\/([^/]+)\/([^/]+)\/tree\/([^/]+)\/(.+)/,
        /github\.com\/([^/]+)\/([^/]+)\/blob\/([^/]+)\/(.+)/,
        /github\.com\/([^/]+)\/([^/]+)(?:\/|$)/,
      ];

      for (const pattern of patterns) {
        const match = url.match(pattern);
        if (match) {
          return {
            owner: match[1],
            repo: match[2].replace('.git', ''),
            ref: match[3] || config.defaultBranch,
            path: match[4] || '',
          };
        }
      }
    } else if (platform === 'gitee') {
      // Gitee URL patterns:
      // https://gitee.com/owner/repo
      // https://gitee.com/owner/repo/tree/branch/path
      // https://gitee.com/owner/repo/blob/branch/path/file
      const patterns = [
        /gitee\.com\/([^/]+)\/([^/]+)\/tree\/([^/]+)\/(.+)/,
        /gitee\.com\/([^/]+)\/([^/]+)\/blob\/([^/]+)\/(.+)/,
        /gitee\.com\/([^/]+)\/([^/]+)(?:\/|$)/,
      ];

      for (const pattern of patterns) {
        const match = url.match(pattern);
        if (match) {
          return {
            owner: match[1],
            repo: match[2].replace('.git', ''),
            ref: match[3] || config.defaultBranch,
            path: match[4] || '',
          };
        }
      }
    }

    return null;
  }, []);

  // Fetch directory contents recursively from Git API
  const fetchGitDirectoryContents = useCallback(
    async (
      platform: GitPlatform,
      owner: string,
      repo: string,
      path: string,
      ref: string,
      token?: string,
    ): Promise<GitFile[]> => {
      const config = PLATFORM_CONFIG[platform];
      const headers: HeadersInit = {
        Accept: 'application/json',
      };

      if (token) {
        if (platform === 'github') {
          headers.Authorization = `token ${token}`;
        } else {
          headers['PRIVATE-TOKEN'] = token;
        }
      }

      let url: string;
      if (platform === 'github') {
        url = `${config.apiBase}/repos/${owner}/${repo}/contents/${path}?ref=${ref}`;
      } else {
        url = `${config.apiBase}/repos/${owner}/${repo}/contents/${path}?ref=${ref}`;
        if (token) {
          url += `&access_token=${token}`;
        }
      }

      const response = await fetch(url, { headers });

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}));
        const message = errorData.message || `HTTP ${response.status}`;

        if (response.status === 403) {
          const limit = platform === 'github' ? '60' : '1000';
          throw new Error(
            `API rate limit exceeded. ${limit} requests/hour for unauthenticated requests.`,
          );
        }
        if (response.status === 404) {
          throw new Error(
            'Repository or path not found. Please check the URL and ensure the repository is public.',
          );
        }
        throw new Error(`Failed to fetch: ${message}`);
      }

      const items = await response.json();
      const files: GitFile[] = [];

      // Handle single file case
      if (!Array.isArray(items)) {
        if (items.type === 'file') {
          files.push({
            path: items.path,
            download_url: items.download_url,
            type: 'file',
            size: items.size,
          });
        }
        return files;
      }

      for (const item of items) {
        if (item.type === 'file') {
          files.push({
            path: item.path,
            download_url: item.download_url,
            type: 'file',
            size: item.size,
          });
        } else if (item.type === 'dir') {
          // Recursively fetch subdirectories
          const subFiles = await fetchGitDirectoryContents(
            platform,
            owner,
            repo,
            item.path,
            ref,
            token,
          );
          files.push(...subFiles);
        }
      }

      return files;
    },
    [],
  );

  // Infer MIME type from file extension
  const getMimeTypeFromExtension = (filePath: string): string => {
    const ext = filePath.split('.').pop()?.toLowerCase() ?? '';
    const mimeTypes: Record<string, string> = {
      md: 'text/markdown',
      mdx: 'text/markdown',
      txt: 'text/plain',
      json: 'application/json',
      json5: 'application/json',
      yaml: 'application/yaml',
      yml: 'application/yaml',
      toml: 'application/toml',
      js: 'application/javascript',
      cjs: 'application/javascript',
      mjs: 'application/javascript',
      ts: 'application/typescript',
      tsx: 'application/typescript',
      jsx: 'application/javascript',
      py: 'text/x-python',
      sh: 'text/x-shellscript',
      rb: 'text/x-ruby',
      go: 'text/x-go',
      rs: 'text/x-rust',
      swift: 'text/x-swift',
      kt: 'text/x-kotlin',
      java: 'text/x-java',
      cs: 'text/x-csharp',
      cpp: 'text/x-c++',
      c: 'text/x-c',
      h: 'text/x-c',
      hpp: 'text/x-c++',
      sql: 'text/x-sql',
      csv: 'text/csv',
      ini: 'text/x-ini',
      cfg: 'text/x-config',
      env: 'text/x-env',
      xml: 'application/xml',
      html: 'text/html',
      htm: 'text/html',
      css: 'text/css',
      scss: 'text/x-scss',
      sass: 'text/x-sass',
      svg: 'image/svg+xml',
    };
    return mimeTypes[ext] || 'text/plain';
  };

  // Download file from Git
  const downloadGitFile = useCallback(
    async (
      platform: GitPlatform,
      file: GitFile,
      owner: string,
      repo: string,
      ref: string,
    ): Promise<File> => {
      let downloadUrl = file.download_url;
      const config = PLATFORM_CONFIG[platform];

      // If download_url is not provided, construct raw URL
      if (!downloadUrl) {
        if (platform === 'github') {
          // https://raw.githubusercontent.com/owner/repo/ref/path
          downloadUrl = `${config.rawBase}/${owner}/${repo}/${ref}/${file.path}`;
        } else if (platform === 'gitee') {
          // https://gitee.com/owner/repo/raw/ref/path
          downloadUrl = `${config.rawBase}/${owner}/${repo}/raw/${ref}/${file.path}`;
        }
      }

      if (!downloadUrl) {
        throw new Error(`Download URL not available for file: ${file.path}`);
      }

      const response = await fetch(downloadUrl);
      if (!response.ok) {
        throw new Error(
          `Failed to download ${file.path}: ${response.status} ${response.statusText}`,
        );
      }

      const blob = await response.blob();
      const fileName = file.path.split('/').pop() || 'file';

      // Use MIME type from extension if blob.type is empty or generic
      let fileType = blob.type;
      if (
        !fileType ||
        fileType === 'application/octet-stream' ||
        fileType === 'text/plain'
      ) {
        fileType = getMimeTypeFromExtension(file.path);
      }

      const downloadedFile = new File([blob], fileName, {
        type: fileType,
      });

      // Add webkitRelativePath to maintain directory structure
      Object.defineProperty(downloadedFile, 'webkitRelativePath', {
        value: file.path,
        writable: false,
      });

      return downloadedFile;
    },
    [],
  );

  // Handle Git import
  const handleGitImport = useCallback(async () => {
    if (!repoUrl || !gitVersion) {
      return;
    }

    if (!validateGitVersion(gitVersion)) {
      setGitValidationStatus('invalid');
      setGitValidationMessage(
        t('skills.versionFormatHelp') ||
          'Version must be in semver format (e.g., 1.0.0)',
      );
      return;
    }

    setGitImporting(true);
    setGitProgress('Parsing repository URL...');
    setGitValidationStatus(null);
    setGitValidationMessage('');

    try {
      const parsed = parseGitUrl(repoUrl, gitPlatform);
      if (!parsed) {
        throw new Error(
          `Invalid ${PLATFORM_CONFIG[gitPlatform].name} URL format`,
        );
      }

      const { owner, repo, ref, path } = parsed;

      // 1. Fetch file list from Git API
      setGitProgress('Fetching file list...');
      const gitFiles = await fetchGitDirectoryContents(
        gitPlatform,
        owner,
        repo,
        path,
        ref,
        gitToken || undefined,
      );

      if (gitFiles.length === 0) {
        throw new Error('No files found in the repository');
      }

      // Filter out common non-skill files
      const filteredGitFiles = gitFiles.filter((f) => {
        const name = f.path.split('/').pop()?.toLowerCase();
        // Skip common non-code files
        if (
          [
            '.gitignore',
            'license',
            'copying',
            'makefile',
            'dockerfile',
          ].includes(name || '')
        ) {
          return false;
        }
        return true;
      });

      // 2. Download all files
      setGitProgress(`Downloading ${filteredGitFiles.length} files...`);
      const downloadedFiles: File[] = [];
      const downloadErrors: string[] = [];

      for (let i = 0; i < filteredGitFiles.length; i++) {
        const file = filteredGitFiles[i];
        setGitProgress(
          `Downloading ${i + 1}/${filteredGitFiles.length}: ${file.path}`,
        );

        try {
          const downloadedFile = await downloadGitFile(
            gitPlatform,
            file,
            owner,
            repo,
            ref,
          );
          downloadedFiles.push(downloadedFile);
        } catch (err) {
          const errorMsg = err instanceof Error ? err.message : String(err);
          console.warn(`Failed to download ${file.path}:`, err);
          downloadErrors.push(`${file.path}: ${errorMsg}`);
        }
      }

      if (downloadedFiles.length === 0) {
        throw new Error(
          `No files could be downloaded. Errors:\n${downloadErrors.slice(0, 3).join('\n')}`,
        );
      }

      // 3. Validate skill format
      setGitProgress('Validating skill format...');

      const validation = await validateSkillFormat(downloadedFiles);

      if (!validation.valid) {
        setGitValidationStatus('invalid');
        const errorKey = `skills.validation.${validation.error}`;
        const errorMessage = t(errorKey) || validation.error;
        const details = validation.details ? `: ${validation.details}` : '';
        setGitValidationMessage(`${errorMessage}${details}`);
        setGitImporting(false);
        setGitProgress('');
        return;
      }

      setGitValidationStatus('valid');
      setGitValidationMessage(
        t('skills.validation.valid') || 'Valid skill format',
      );

      // 4. Upload to RAGFlow
      setGitProgress('Uploading to RAGFlow...');
      const skillName =
        validation.name || repo.toLowerCase().replace(/[^a-z0-9_-]/g, '-');

      const success = await onUpload(skillName, gitVersion, downloadedFiles);

      if (success) {
        handleCancel();
      }
    } catch (error) {
      console.error('Git import error:', error);
      setGitValidationStatus('invalid');
      setGitValidationMessage(
        error instanceof Error ? error.message : 'Import failed',
      );
    } finally {
      setGitImporting(false);
      setGitProgress('');
    }
  }, [
    repoUrl,
    gitVersion,
    gitPlatform,
    gitToken,
    t,
    parseGitUrl,
    fetchGitDirectoryContents,
    downloadGitFile,
    onUpload,
    handleCancel,
  ]);

  // Check if Git import can be submitted
  const isGitImportDisabled =
    !repoUrl || !gitVersion || !validateGitVersion(gitVersion) || gitImporting;

  // Handle tab change
  const handleTabChange = (value: string) => {
    setActiveTab(value);
  };

  return (
    <Modal
      open={open}
      onOpenChange={(v: boolean) => !v && handleCancel()}
      title={t('skills.addSkill') || 'Add Skill'}
      showfooter={false}
      onCancel={handleCancel}
      size="large"
    >
      <Tabs value={activeTab} onValueChange={handleTabChange} className="mt-4">
        <TabsList className="grid w-full grid-cols-2">
          <TabsTrigger value="upload" disabled={gitImporting}>
            <FolderOpen className="mr-2 size-4" />
            {t('skills.upload') || 'Upload'}
          </TabsTrigger>
          <TabsTrigger value="git" disabled={uploading}>
            <Globe className="mr-2 size-4" />
            {t('skills.importFromGit') || 'Import from Git'}
          </TabsTrigger>
        </TabsList>

        {/* Upload Tab */}
        <TabsContent value="upload" className="space-y-4 mt-4">
          <Form {...form}>
            <RAGFlowFormItem
              name="name"
              label={
                <>
                  {t('skills.skillName')}
                  <span className="text-state-error ml-1">*</span>
                </>
              }
              rules={{ validate: validateName }}
            >
              <Input
                id="skill-name"
                placeholder={t('skills.skillNamePlaceholder')}
                disabled={uploading}
                {...register('name', { validate: validateName })}
              />
            </RAGFlowFormItem>

            <RAGFlowFormItem
              name="version"
              label={
                <>
                  {t('skills.skillVersion') || 'Version'}
                  <span className="text-state-error ml-1">*</span>
                </>
              }
              rules={{ validate: validateVersion }}
            >
              <Input
                id="skill-version"
                placeholder={
                  t('skills.skillVersionPlaceholder') || 'e.g., 1.0.0'
                }
                disabled={uploading}
                {...register('version', { validate: validateVersion })}
              />
            </RAGFlowFormItem>
            <p className="text-xs text-text-secondary">
              {t('skills.versionFormatHelp') ||
                'Version must be in semver format (e.g., 1.0.0)'}
            </p>
          </Form>

          <div className="bg-bg-card border border-border-button rounded-lg p-4">
            <p className="font-medium text-sm">
              {t('skills.selectFilesOrFolder')}
            </p>
            <p className="text-text-secondary text-sm mt-1">
              {t('skills.uploadDescription')}
            </p>
          </div>

          {/* File Uploader */}
          <FileUploader
            value={files}
            onValueChange={handleFilesChange}
            disabled={uploading}
            multiple
            accept={{ '*/*': [] }}
            title={t('skills.dragFilesTitle')}
            description={t('skills.dragFilesDescription')}
          />

          {/* Validation Status */}
          {validationStatus && (
            <div
              className={`border rounded-lg p-4 ${
                validationStatus === 'valid'
                  ? 'bg-state-success/5 border-state-success/20'
                  : validationStatus === 'invalid'
                    ? 'bg-state-error/5 border-state-error/20'
                    : 'bg-bg-card border-border-button'
              }`}
            >
              <div className="flex items-start gap-3">
                {validationStatus === 'valid' ? (
                  <CheckCircle className="size-5 text-state-success flex-shrink-0 mt-0.5" />
                ) : validationStatus === 'invalid' ? (
                  <XCircle className="size-5 text-state-error flex-shrink-0 mt-0.5" />
                ) : null}
                <div className="flex-1">
                  <p
                    className={`font-medium ${
                      validationStatus === 'valid'
                        ? 'text-state-success'
                        : validationStatus === 'invalid'
                          ? 'text-state-error'
                          : 'text-text-primary'
                    }`}
                  >
                    {validationStatus === 'valid'
                      ? t('skills.validation.valid') || 'Valid skill format'
                      : t('skills.validation.invalid') ||
                        'Invalid skill format'}
                  </p>
                  <p className="text-text-secondary text-sm mt-1">
                    {validationMessage}
                  </p>
                  {parsedMetadata && (
                    <div className="mt-3 pt-3 border-t border-border-button">
                      <p className="text-text-secondary text-sm font-medium">
                        {t('skills.parsedMetadata') || 'Parsed from SKILL.md:'}
                      </p>
                      {parsedMetadata.name && (
                        <div className="text-sm mt-1">
                          <span className="text-text-secondary">
                            {t('skills.name') || 'Name'}:{' '}
                          </span>
                          <span>{parsedMetadata.name}</span>
                        </div>
                      )}
                      {parsedMetadata.description && (
                        <div className="text-sm mt-1">
                          <span className="text-text-secondary">
                            {t('skills.description') || 'Description'}:{' '}
                          </span>
                          <span>
                            {parsedMetadata.description.slice(0, 100)}
                            {parsedMetadata.description.length > 100
                              ? '...'
                              : ''}
                          </span>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              </div>
            </div>
          )}

          {uploading && progress > 0 && (
            <div className="space-y-2">
              <Progress value={progress} />
              <p className="text-text-secondary text-sm text-center">
                {t('skills.uploading')}...
              </p>
            </div>
          )}

          {/* Upload Actions */}
          <div className="flex justify-end gap-2 pt-4 border-t border-border-button">
            <Button
              variant="outline"
              onClick={handleCancel}
              disabled={uploading}
            >
              {t('common.cancel')}
            </Button>
            <Button
              onClick={handleOk}
              disabled={isUploadDisabled || uploading}
              loading={uploading}
            >
              {uploading ? t('skills.uploading') : t('common.upload')}
            </Button>
          </div>
        </TabsContent>

        {/* Git Import Tab */}
        <TabsContent value="git" className="space-y-4 mt-4">
          {/* Platform Selection */}
          <div className="space-y-2">
            <Label>{t('skills.gitPlatform') || 'Platform'}</Label>
            <div className="flex gap-2">
              <Button
                type="button"
                variant={gitPlatform === 'github' ? 'default' : 'outline'}
                onClick={() => setGitPlatform('github')}
                disabled={gitImporting}
                className="flex-1"
              >
                <Globe className="mr-2 size-4" />
                GitHub
              </Button>
              <Button
                type="button"
                variant={gitPlatform === 'gitee' ? 'default' : 'outline'}
                onClick={() => setGitPlatform('gitee')}
                disabled={gitImporting}
                className="flex-1"
              >
                <Globe className="mr-2 size-4" />
                Gitee
              </Button>
            </div>
          </div>

          {/* Repository URL */}
          <div className="space-y-2">
            <Label htmlFor="git-repo-url">
              {t('skills.repoUrl') || 'Repository URL'}
              <span className="text-state-error ml-1">*</span>
            </Label>
            <Input
              id="git-repo-url"
              placeholder={
                gitPlatform === 'github'
                  ? 'https://github.com/owner/repo/tree/main/skill-path'
                  : 'https://gitee.com/owner/repo/tree/master/skill-path'
              }
              disabled={gitImporting}
              value={repoUrl}
              onChange={(e) => setRepoUrl(e.target.value)}
            />
            <p className="text-xs text-text-secondary">
              {t('skills.repoUrlHelp') ||
                `Supports: ${PLATFORM_CONFIG[gitPlatform].name} repository URL with optional path`}
            </p>
          </div>

          {/* Version */}
          <div className="space-y-2">
            <Label htmlFor="git-version">
              {t('skills.skillVersion') || 'Version'}
              <span className="text-state-error ml-1">*</span>
            </Label>
            <Input
              id="git-version"
              placeholder="1.0.0"
              disabled={gitImporting}
              value={gitVersion}
              onChange={(e) => setGitVersion(e.target.value)}
            />
            <p className="text-xs text-text-secondary">
              {t('skills.versionFormatHelp') ||
                'Version must be in semver format (e.g., 1.0.0)'}
            </p>
          </div>

          {/* Access Token (Optional) */}
          <div className="space-y-2">
            <Label htmlFor="git-token">
              {t('skills.accessToken') || 'Access Token'}
              <span className="text-text-secondary ml-1">
                ({t('common.optional') || 'optional'})
              </span>
            </Label>
            <Input
              id="git-token"
              type="password"
              placeholder={
                gitPlatform === 'github' ? 'ghp_xxxxxxxxxxxx' : 'gitee token'
              }
              disabled={gitImporting}
              value={gitToken}
              onChange={(e) => setGitToken(e.target.value)}
            />
            <p className="text-xs text-text-secondary">
              {gitPlatform === 'github'
                ? t('skills.githubTokenHelp') ||
                  'For private repos or higher rate limits (5000 req/hour)'
                : t('skills.giteeTokenHelp') ||
                  'For private repos or higher rate limits (2000 req/hour)'}
            </p>
          </div>

          {/* Rate Limit Info */}
          <div className="bg-bg-card border border-border-button rounded-lg p-4">
            <p className="text-sm font-medium">
              {t('skills.rateLimitInfo') || 'Rate Limit Info'}
            </p>
            <p className="text-text-secondary text-sm mt-1">
              {gitPlatform === 'github'
                ? t('skills.githubRateLimit') ||
                  'Public repos: 60 requests/hour per IP. Use token for 5000 req/hour.'
                : t('skills.giteeRateLimit') ||
                  'Public repos: 1000 requests/hour per IP. Use token for 2000 req/hour.'}
            </p>
          </div>

          {/* Progress */}
          {gitImporting && gitProgress && (
            <div className="bg-bg-card border border-border-button rounded-lg p-4">
              <div className="flex items-center gap-3">
                <Loader2 className="size-5 animate-spin text-accent-primary" />
                <span className="text-sm">{gitProgress}</span>
              </div>
            </div>
          )}

          {/* Validation Status */}
          {gitValidationStatus && (
            <div
              className={`border rounded-lg p-4 ${
                gitValidationStatus === 'valid'
                  ? 'bg-state-success/5 border-state-success/20'
                  : 'bg-state-error/5 border-state-error/20'
              }`}
            >
              <div className="flex items-start gap-3">
                {gitValidationStatus === 'valid' ? (
                  <CheckCircle className="size-5 text-state-success flex-shrink-0 mt-0.5" />
                ) : (
                  <XCircle className="size-5 text-state-error flex-shrink-0 mt-0.5" />
                )}
                <div className="flex-1">
                  <p
                    className={`font-medium ${
                      gitValidationStatus === 'valid'
                        ? 'text-state-success'
                        : 'text-state-error'
                    }`}
                  >
                    {gitValidationStatus === 'valid'
                      ? t('skills.validation.valid') || 'Valid'
                      : t('skills.validation.invalid') || 'Error'}
                  </p>
                  <p className="text-text-secondary text-sm mt-1">
                    {gitValidationMessage}
                  </p>
                </div>
              </div>
            </div>
          )}

          {/* Git Import Actions */}
          <div className="flex justify-end gap-2 pt-4 border-t border-border-button">
            <Button
              variant="outline"
              onClick={handleCancel}
              disabled={gitImporting}
            >
              {t('common.cancel')}
            </Button>
            <Button
              onClick={handleGitImport}
              disabled={isGitImportDisabled}
              loading={gitImporting}
            >
              {gitImporting
                ? t('skills.importing') || 'Importing...'
                : t('skills.import') || 'Import'}
            </Button>
          </div>
        </TabsContent>
      </Tabs>
    </Modal>
  );
};

export default UploadModal;
