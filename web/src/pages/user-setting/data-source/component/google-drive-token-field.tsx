import { useMemo, useState } from 'react';

import { FileUploader } from '@/components/file-uploader';
import message from '@/components/ui/message';
import { Textarea } from '@/components/ui/textarea';
import { FileMimeType } from '@/constants/common';

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
  const [files, setFiles] = useState<File[]>([]);

  const handleValueChange = useMemo(
    () => (nextFiles: File[]) => {
      if (!nextFiles.length) {
        setFiles([]);
        return;
      }
      const file = nextFiles[nextFiles.length - 1];
      file
        .text()
        .then((text) => {
          JSON.parse(text);
          onChange(text);
          setFiles([file]);
          message.success('JSON uploaded');
        })
        .catch(() => {
          message.error('Invalid JSON file.');
          setFiles([]);
        });
    },
    [onChange],
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
        className="min-h-[120px] max-h-60 overflow-y-auto"
      />
      <FileUploader
        className="py-4"
        value={files}
        onValueChange={handleValueChange}
        accept={{ '*.json': [FileMimeType.Json] }}
        maxFileCount={1}
      />
    </div>
  );
};

export default GoogleDriveTokenField;
