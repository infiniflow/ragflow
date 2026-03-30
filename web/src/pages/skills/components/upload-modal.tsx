import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Modal } from '@/components/ui/modal/modal';
import { Progress } from '@/components/ui/progress';
import { CheckCircle, File, FolderOpen, Inbox, XCircle } from 'lucide-react';
import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { validateSkillFormat } from '../hooks';
import type { ValidationError } from '../types';

interface UploadFile {
  uid: string;
  name: string;
  size?: number;
  type?: string;
  originFileObj?: File;
  status?: 'done' | 'error' | 'uploading';
}

interface UploadModalProps {
  open: boolean;
  onCancel: () => void;
  onUpload: (name: string, files: File[]) => Promise<boolean>;
  loading?: boolean;
}

type FileWithPath = File & {
  webkitRelativePath?: string;
};

const UploadModal: React.FC<UploadModalProps> = ({
  open,
  onCancel,
  onUpload,
  loading,
}) => {
  const { t } = useTranslation();
  const [name, setName] = useState('');
  const [nameError, setNameError] = useState('');
  const [fileList, setFileList] = useState<UploadFile[]>([]);
  const [uploading, setUploading] = useState(false);
  const [progress, setProgress] = useState(0);
  const [inputKey, setInputKey] = useState(0); // Used to force re-render file input
  const [validationStatus, setValidationStatus] = useState<
    'valid' | 'invalid' | 'pending' | null
  >(null);
  const [validationMessage, setValidationMessage] = useState<string>('');
  const [, setValidationErrors] = useState<ValidationError[]>([]);
  const [parsedMetadata, setParsedMetadata] = useState<{
    name?: string;
    description?: string;
  } | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const validateName = (value: string): boolean => {
    if (!value) {
      setNameError(t('skills.skillNameHelp'));
      return false;
    }
    if (!/^[a-zA-Z0-9_-]+$/.test(value)) {
      setNameError(t('skills.skillNameHelp'));
      return false;
    }
    setNameError('');
    return true;
  };

  const handleOk = useCallback(async () => {
    if (!validateName(name)) {
      return;
    }

    if (fileList.length === 0) {
      return;
    }

    setUploading(true);
    setProgress(0);

    // Get actual File objects
    const files: File[] = fileList
      .map((f) => f.originFileObj)
      .filter(Boolean) as File[];

    try {
      const success = await onUpload(name, files);

      if (success) {
        setName('');
        setFileList([]);
        onCancel();
      }
    } catch (error) {
      console.error('Upload error:', error);
    } finally {
      setUploading(false);
      setProgress(0);
    }
  }, [name, fileList, onUpload, onCancel, t]);

  const handleCancel = useCallback(() => {
    if (!uploading) {
      setName('');
      setNameError('');
      setFileList([]);
      setValidationStatus(null);
      setValidationMessage('');
      setValidationErrors([]);
      setParsedMetadata(null);
      onCancel();
    }
  }, [uploading, onCancel]);

  // Handle folder drop from drag and drop
  const handleFolderDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();

      const items = e.dataTransfer.items;
      if (!items || items.length === 0) return;

      const files: File[] = [];
      const promises: Promise<void>[] = [];

      // Process each dropped item
      for (let i = 0; i < items.length; i++) {
        const item = items[i];
        if (item.kind === 'file') {
          const entry =
            item.webkitGetAsEntry?.() || (item as any).getAsEntry?.();
          if (entry && entry.isDirectory) {
            // Recursively read directory
            promises.push(readDirectory(entry, files, ''));
          } else if (entry && entry.isFile) {
            promises.push(
              new Promise((resolve) => {
                (entry as FileSystemFileEntry).file((file) => {
                  // Add webkitRelativePath
                  Object.defineProperty(file, 'webkitRelativePath', {
                    value: file.name,
                    writable: false,
                  });
                  files.push(file);
                  resolve();
                });
              }),
            );
          }
        }
      }

      Promise.all(promises)
        .then(() => {
          if (files.length > 0) {
            const uploadFiles: UploadFile[] = files.map((file, index) => ({
              uid: `${Date.now()}-${index}`,
              name: (file as any).webkitRelativePath || file.name,
              size: file.size,
              type: file.type,
              originFileObj: file as any,
              status: 'done',
            }));

            setFileList(uploadFiles);

            // Auto-fill name from folder name
            const folderName =
              (files[0] as any).webkitRelativePath?.split('/')[0] ||
              files[0].name;
            if (folderName && !name) {
              setName(folderName);
            }
          }
        })
        .catch((err) => {
          console.error('Error processing dropped folder:', err);
        });
    },
    [name],
  );

  // Recursively read directory contents
  const readDirectory = (
    dirEntry: FileSystemDirectoryEntry,
    files: File[],
    path: string,
  ): Promise<void> => {
    return new Promise((resolve, reject) => {
      const reader = dirEntry.createReader();
      const entries: FileSystemEntry[] = [];

      const readEntries = () => {
        reader.readEntries((results) => {
          if (results.length === 0) {
            // Process all entries
            const promises: Promise<void>[] = [];
            for (const entry of entries) {
              const newPath = path ? `${path}/${entry.name}` : entry.name;
              if (entry.isDirectory) {
                promises.push(
                  readDirectory(
                    entry as FileSystemDirectoryEntry,
                    files,
                    newPath,
                  ),
                );
              } else if (entry.isFile) {
                promises.push(
                  new Promise((resolveFile) => {
                    (entry as FileSystemFileEntry).file((file) => {
                      Object.defineProperty(file, 'webkitRelativePath', {
                        value: newPath,
                        writable: false,
                      });
                      files.push(file);
                      resolveFile();
                    });
                  }),
                );
              }
            }
            Promise.all(promises)
              .then(() => resolve())
              .catch(reject);
          } else {
            entries.push(...results);
            readEntries();
          }
        }, reject);
      };

      readEntries();
    });
  };

  // Prevent default drag behavior
  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
  }, []);

  // Validate files when fileList changes
  useEffect(() => {
    const validateFiles = async () => {
      if (fileList.length === 0) {
        setValidationStatus(null);
        setValidationMessage('');
        setValidationErrors([]);
        setParsedMetadata(null);
        return;
      }

      setValidationStatus('pending');

      try {
        const files: File[] = fileList
          .map((f) => f.originFileObj)
          .filter(Boolean) as File[];

        if (files.length === 0) {
          setValidationStatus(null);
          setValidationMessage('');
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
          if (result.name && !name) {
            setName(result.name);
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
        setValidationMessage(
          t('skills.validation.invalid') || 'Invalid skill format',
        );
        setValidationErrors([]);
        setParsedMetadata(null);
      }
    };

    validateFiles();
  }, [fileList, t, name]);

  // Handle folder selection via webkitdirectory
  const handleFolderSelect = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const files = e.target.files;
      if (!files || files.length === 0) {
        // Force re-render input to allow selecting the same folder again
        setInputKey((prev) => prev + 1);
        return;
      }

      const fileArray = Array.from(files).map((file, index): UploadFile => {
        const fileWithPath = file as FileWithPath;
        return {
          uid: `${Date.now()}-${index}`,
          name: fileWithPath.webkitRelativePath || file.name,
          size: file.size,
          type: file.type,
          originFileObj: fileWithPath as any,
          status: 'done',
        };
      });

      setFileList((prev) => [...prev, ...fileArray]);

      // Auto-fill name from folder name if empty
      const folderName = fileArray[0]?.name?.split('/')[0];
      if (folderName && !name) {
        setName(folderName);
      }

      // Force re-render input to allow selecting the same folder again
      setInputKey((prev) => prev + 1);
    },
    [name],
  );

  const handleRemoveFile = (uid: string) => {
    setFileList((prev) => prev.filter((f) => f.uid !== uid));
  };

  const fileCount = fileList.length;
  const folderCount = new Set(fileList.map((f) => f.name.split('/')[0])).size;

  const isUploadDisabled =
    validationStatus === 'invalid' || fileList.length === 0;

  return (
    <Modal
      open={open}
      onOpenChange={(v: boolean) => !v && handleCancel()}
      title={t('skills.uploadSkill')}
      showfooter={true}
      onOk={handleOk}
      onCancel={handleCancel}
      confirmLoading={uploading || loading}
      okText={uploading ? t('skills.uploading') : t('common.upload')}
      cancelText={t('common.cancel')}
      disabled={isUploadDisabled}
      size="default"
    >
      <div className="space-y-4 mt-4">
        <div className="space-y-2">
          <Label htmlFor="skill-name">
            {t('skills.skillName')}
            <span className="text-state-error ml-1">*</span>
          </Label>
          <Input
            id="skill-name"
            placeholder={t('skills.skillNamePlaceholder')}
            disabled={uploading}
            value={name}
            onChange={(e) => {
              setName(e.target.value);
              if (nameError) validateName(e.target.value);
            }}
          />
          {nameError && <p className="text-sm text-state-error">{nameError}</p>}
        </div>

        <div className="bg-bg-card border border-border-button rounded-lg p-4">
          <p className="font-medium text-sm">
            {t('skills.selectFilesOrFolder')}
          </p>
          <p className="text-text-secondary text-sm mt-1">
            {t('skills.uploadDescription')}
          </p>
        </div>

        {/* Folder Upload Button */}
        <div className="flex items-center gap-2">
          <input
            key={inputKey}
            type="file"
            ref={fileInputRef}
            className="hidden"
            // @ts-ignore - webkitdirectory is not standard but widely supported
            webkitdirectory="true"
            directory="true"
            onChange={handleFolderSelect}
          />
          <Button
            type="button"
            variant="outline"
            onClick={() => fileInputRef.current?.click()}
            disabled={uploading}
          >
            <FolderOpen className="mr-2 size-4" />
            {t('skills.selectFolder')}
          </Button>
          <span className="text-text-secondary text-sm">
            {t('skills.dragFilesHint')}
          </span>
        </div>

        {/* Drag and Drop Zone */}
        <div
          className="border-2 border-dashed border-border-button rounded-lg p-8 text-center hover:border-accent-primary transition-colors"
          onDrop={handleFolderDrop}
          onDragOver={handleDragOver}
          onDragEnter={handleDragOver}
        >
          <Inbox className="mx-auto size-10 text-text-secondary mb-3" />
          <p className="text-text-primary font-medium">
            {t('skills.dragFilesTitle')}
          </p>
          <p className="text-text-secondary text-sm mt-1">
            {t('skills.dragFilesDescription')}
          </p>
        </div>

        {/* File List */}
        {fileList.length > 0 && (
          <div className="border border-border-button rounded-lg p-3 max-h-40 overflow-y-auto">
            <div className="flex items-center gap-2 text-text-secondary text-sm mb-2">
              <File className="size-4" />
              <span>
                {t('skills.filesSelected', { count: fileCount })}
                {folderCount > 1 && ` (${folderCount} folders)`}
              </span>
            </div>
            <div className="space-y-1">
              {fileList.map((file) => (
                <div
                  key={file.uid}
                  className="flex items-center justify-between text-sm py-1 px-2 hover:bg-bg-card rounded"
                >
                  <span className="truncate flex-1">{file.name}</span>
                  {!uploading && (
                    <button
                      onClick={() => handleRemoveFile(file.uid)}
                      className="text-text-secondary hover:text-state-error ml-2"
                    >
                      <XCircle className="size-4" />
                    </button>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}

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
                    : t('skills.validation.invalid') || 'Invalid skill format'}
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
                          {parsedMetadata.description.length > 100 ? '...' : ''}
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
      </div>
    </Modal>
  );
};

export default UploadModal;
