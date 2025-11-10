import { IDocumentInfo } from '@/interfaces/database/document';
import { getExtension } from '@/utils/document-util';
import { formatBytes } from '@/utils/file-util';
import { memo } from 'react';
import FileIcon from '../file-icon';
import NewDocumentLink from '../new-document-link';
import SvgIcon from '../svg-icon';

interface IProps {
  files?: File[] | IDocumentInfo[];
}

type NameWidgetType = {
  name: string;
  size: number;
  id?: string;
};
function NameWidget({ name, size, id }: NameWidgetType) {
  return (
    <div className="text-xs max-w-20">
      {id ? (
        <NewDocumentLink documentId={id} documentName={name} prefix="document">
          {name}
        </NewDocumentLink>
      ) : (
        <div className="truncate">{name}</div>
      )}
      <p className="text-text-secondary pt-1">{formatBytes(size)}</p>
    </div>
  );
}
export function InnerUploadedMessageFiles({ files = [] }: IProps) {
  return (
    <section className="flex gap-2 pt-2">
      {files?.map((file, idx) => {
        const name = file.name;
        const isFile = file instanceof File;

        return (
          <div key={idx} className="flex gap-1 border rounded-md p-1.5">
            {!isFile ? (
              <FileIcon id={file.id} name={name}></FileIcon>
            ) : file.type.startsWith('image/') ? (
              <img
                src={URL.createObjectURL(file)}
                alt={name}
                className="size-10 object-cover"
              />
            ) : (
              <SvgIcon
                name={`file-icon/${getExtension(name)}`}
                width={24}
              ></SvgIcon>
            )}
            <NameWidget
              name={name}
              size={file.size}
              id={isFile ? undefined : file.id}
            ></NameWidget>
          </div>
        );
      })}
    </section>
  );
}

export const UploadedMessageFiles = memo(InnerUploadedMessageFiles);
