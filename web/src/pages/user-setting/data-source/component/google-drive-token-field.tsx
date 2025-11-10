import { useMemo, useState } from 'react';

import message from '@/components/ui/message';
import { Textarea } from '@/components/ui/textarea';
import { InboxOutlined } from '@ant-design/icons';
import { RcFile, Upload, UploadFile, UploadProps } from 'antd';

type GoogleDriveTokenFieldProps = {
  value?: string;
  onChange: (value: any) => void;
  placeholder?: string;
};

const GoogleDriveTokenField = ({
  value,
  onChange,
  placeholder,
}: GoogleDriveTokenFieldProps) => {
  const [fileList, setFileList] = useState<UploadFile[]>([]);

  const handleFile = async (file: RcFile) => {
    try {
      const text = await file.text();
      JSON.parse(text);
      onChange(text);
      message.success('JSON uploaded');
      setFileList([file as UploadFile]);
    } catch (error) {
      message.error('Invalid JSON file.');
      return Upload.LIST_IGNORE;
    } finally {
      // noop
    }
    return false;
  };

  const uploadProps: UploadProps = useMemo(
    () => ({
      accept: '.json,application/json',
      maxCount: 1,
      multiple: false,
      beforeUpload: handleFile,
      onRemove: () => {
        setFileList([]);
      },
      fileList,
      showUploadList: true,
    }),
    [fileList],
  );

  return (
    <div className="flex flex-col gap-2">
      <Textarea
        value={value || ''}
        onChange={(event) => onChange(event.target.value)}
        placeholder={
          placeholder ||
          '{ "token": "...", "refresh_token": "...", "client_id": "...", ... }'
        }
        className="min-h-[120px]"
      />
      <Upload.Dragger {...uploadProps} className="!py-4">
        <p className="ant-upload-drag-icon">
          <InboxOutlined />
        </p>
        <p className="ant-upload-text">Click or drag a JSON file to upload</p>
        <p className="ant-upload-hint">
          The file content will be stored directly as OAuth tokens.
        </p>
      </Upload.Dragger>
    </div>
  );
};

export default GoogleDriveTokenField;
