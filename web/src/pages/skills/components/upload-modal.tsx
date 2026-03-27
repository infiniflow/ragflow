import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  FileOutlined,
  FolderOpenOutlined,
  InboxOutlined,
} from '@ant-design/icons';
import {
  Alert,
  Button,
  Form,
  Input,
  message,
  Modal,
  Progress,
  Typography,
  Upload,
} from 'antd';
import type { UploadFile, UploadProps } from 'antd/es/upload';
import React, { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { validateSkillFormat } from '../hooks';

const { Dragger } = Upload;
const { Text } = Typography;

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
  const [form] = Form.useForm();
  const [fileList, setFileList] = useState<UploadFile[]>([]);
  const [uploading, setUploading] = useState(false);
  const [progress, setProgress] = useState(0);
  const [inputKey, setInputKey] = useState(0); // Used to force re-render file input
  const [validationStatus, setValidationStatus] = useState<
    'valid' | 'invalid' | 'pending' | null
  >(null);
  const [validationMessage, setValidationMessage] = useState<string>('');

  const handleOk = useCallback(async () => {
    try {
      const values = await form.validateFields();

      if (fileList.length === 0) {
        message.error(t('skills.selectFilesOrFolder'));
        return;
      }

      setUploading(true);
      setProgress(0);

      // Get actual File objects
      const files: File[] = fileList
        .map((f) => f.originFileObj)
        .filter(Boolean) as File[];

      const success = await onUpload(values.name, files);

      if (success) {
        form.resetFields();
        setFileList([]);
        onCancel();
      }
    } catch (error) {
      console.error('Upload error:', error);
    } finally {
      setUploading(false);
      setProgress(0);
    }
  }, [form, fileList, onUpload, onCancel, t]);

  const handleCancel = useCallback(() => {
    if (!uploading) {
      form.resetFields();
      setFileList([]);
      setValidationStatus(null);
      setValidationMessage('');
      onCancel();
    }
  }, [uploading, form, onCancel]);

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
            if (folderName && !form.getFieldValue('name')) {
              form.setFieldsValue({ name: folderName });
            }
          }
        })
        .catch((err) => {
          console.error('Error processing dropped folder:', err);
          message.error('Failed to process dropped folder');
        });
    },
    [form],
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

  const uploadProps: UploadProps = {
    onRemove: (file) => {
      const index = fileList.indexOf(file);
      const newFileList = fileList.slice();
      newFileList.splice(index, 1);
      setFileList(newFileList);
    },
    beforeUpload: () => {
      // Disable default upload behavior, we handle it manually
      return false;
    },
    fileList,
    multiple: false,
    showUploadList: {
      showRemoveIcon: !uploading,
    },
    // Disable click to select - use button instead
    openFileDialogOnClick: false,
  };

  // Validate files when fileList changes
  useEffect(() => {
    const validateFiles = async () => {
      if (fileList.length === 0) {
        setValidationStatus(null);
        setValidationMessage('');
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
          return;
        }

        const result = await validateSkillFormat(files);

        if (result.valid) {
          setValidationStatus('valid');
          setValidationMessage(
            t('skills.validation.valid') || 'Valid skill format',
          );
        } else {
          setValidationStatus('invalid');
          const errorKey = `skills.validation.${result.error}`;
          setValidationMessage(t(errorKey) || t('skills.validation.invalid'));
        }
      } catch (err) {
        console.error('Validation error:', err);
        setValidationStatus('invalid');
        setValidationMessage(
          t('skills.validation.invalid') || 'Invalid skill format',
        );
      }
    };

    validateFiles();
  }, [fileList, t]);

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
      if (folderName && !form.getFieldValue('name')) {
        form.setFieldsValue({ name: folderName });
      }

      // Force re-render input to allow selecting the same folder again
      setInputKey((prev) => prev + 1);
    },
    [form],
  );

  const fileCount = fileList.length;
  const folderCount = new Set(fileList.map((f) => f.name.split('/')[0])).size;

  const isUploadDisabled =
    validationStatus === 'invalid' || fileList.length === 0;

  return (
    <Modal
      title={t('skills.uploadSkill')}
      open={open}
      onOk={handleOk}
      onCancel={handleCancel}
      confirmLoading={uploading || loading}
      okText={uploading ? t('skills.uploading') : t('common.upload')}
      cancelText={t('common.cancel')}
      okButtonProps={{ disabled: isUploadDisabled }}
      width={600}
    >
      <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
        <Form.Item
          name="name"
          label={t('skills.skillName')}
          rules={[
            { required: true, message: t('skills.skillNameHelp') },
            { pattern: /^[a-zA-Z0-9_-]+$/, message: t('skills.skillNameHelp') },
          ]}
        >
          <Input
            placeholder={t('skills.skillNamePlaceholder')}
            disabled={uploading}
          />
        </Form.Item>

        <Alert
          message={t('skills.selectFilesOrFolder')}
          description={t('skills.uploadDescription')}
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
        />

        {/* Folder Upload Button */}
        <div style={{ marginBottom: 16 }}>
          <input
            key={inputKey}
            type="file"
            id="folder-upload"
            style={{ display: 'none' }}
            // @ts-ignore - webkitdirectory is not standard but widely supported
            webkitdirectory="true"
            directory="true"
            onChange={handleFolderSelect}
          />
          <Button
            icon={<FolderOpenOutlined />}
            onClick={() => document.getElementById('folder-upload')?.click()}
            disabled={uploading}
          >
            {t('skills.selectFolder')}
          </Button>
          <Text type="secondary" style={{ marginLeft: 8 }}>
            {t('skills.dragFilesHint')}
          </Text>
        </div>

        <div
          onDrop={handleFolderDrop}
          onDragOver={handleDragOver}
          onDragEnter={handleDragOver}
        >
          <Dragger {...uploadProps} disabled={uploading}>
            <p className="ant-upload-drag-icon">
              <InboxOutlined />
            </p>
            <p className="ant-upload-text">{t('skills.dragFilesTitle')}</p>
            <p className="ant-upload-hint">
              {t('skills.dragFilesDescription')}
            </p>
          </Dragger>
        </div>

        {fileCount > 0 && (
          <div style={{ marginTop: 16 }}>
            <Text type="secondary">
              <FileOutlined style={{ marginRight: 8 }} />
              {t('skills.filesSelected', { count: fileCount })}
              {folderCount > 1 && ` (${folderCount} folders)`}
            </Text>
          </div>
        )}

        {validationStatus && (
          <Alert
            message={
              validationStatus === 'valid'
                ? t('skills.validation.valid') || 'Valid skill format'
                : t('skills.validation.invalid') || 'Invalid skill format'
            }
            description={validationMessage}
            type={
              validationStatus === 'valid'
                ? 'success'
                : validationStatus === 'invalid'
                  ? 'error'
                  : 'info'
            }
            showIcon
            icon={
              validationStatus === 'valid' ? (
                <CheckCircleOutlined />
              ) : validationStatus === 'invalid' ? (
                <CloseCircleOutlined />
              ) : undefined
            }
            style={{ marginTop: 16 }}
          />
        )}

        {uploading && progress > 0 && (
          <Progress
            percent={progress}
            status="active"
            style={{ marginTop: 16 }}
          />
        )}
      </Form>
    </Modal>
  );
};

export default UploadModal;
