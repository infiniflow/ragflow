import { getExtension } from '@/utils/document-util';
import { formatBytes } from '@/utils/file-util';
import { memo } from 'react';
import SvgIcon from '../svg-icon';

interface IProps {
  files?: File[];
}
export function InnerUploadedMessageFiles({ files = [] }: IProps) {
  return (
    <section className="flex gap-2 pt-2">
      {files?.map((file, idx) => (
        <div key={idx} className="flex gap-1 border rounded-md p-1.5">
          {file.type.startsWith('image/') ? (
            <img
              src={URL.createObjectURL(file)}
              alt={file.name}
              className="size-10 object-cover"
            />
          ) : (
            <SvgIcon
              name={`file-icon/${getExtension(file.name)}`}
              width={24}
            ></SvgIcon>
          )}
          <div className="text-xs max-w-20">
            <div className="truncate">{file.name}</div>
            <p className="text-text-sub-title pt-1">{formatBytes(file.size)}</p>
          </div>
        </div>
      ))}
    </section>
  );
}

export const UploadedMessageFiles = memo(InnerUploadedMessageFiles);
