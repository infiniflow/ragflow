/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { UploadResponseDataType } from '@/interfaces/database/chat';
import { IDocumentInfo } from '@/interfaces/database/document';
import api from '@/utils/api';
import { getExtension } from '@/utils/document-util';
import { formatBytes } from '@/utils/file-util';
import { memo, useEffect, useState } from 'react';
import { PhotoProvider, PhotoView } from 'react-photo-view';
import FileIcon from '../file-icon';
import { useAuthenticatedImageUrl } from '../image';
import SvgIcon from '../svg-icon';
import { getFileMimeType, isImageFile } from './utils';

interface IProps {
  files?: File[] | IDocumentInfo[] | UploadResponseDataType[];
}

// Zoomable thumbnail for files already uploaded to the server: chat uploads
// are stored as blobs in the per-user downloads bucket (no file/document
// row), so the raw content is fetched through the authenticated attachment
// preview endpoint and displayed in a lightbox (react-photo-view) on click.
function UploadedFileImage({
  id,
  name,
  mimeType,
}: {
  id: string;
  name: string;
  mimeType?: string;
}) {
  const src = useAuthenticatedImageUrl(
    id
      ? api.getAttachmentFilePreview({ docId: id, filename: name, mimeType })
      : null,
  );

  if (!src) {
    return <FileIcon id={id} name={name}></FileIcon>;
  }

  return (
    <PhotoView src={src}>
      <img
        src={src}
        alt={name}
        className="size-10 object-cover cursor-zoom-in"
      />
    </PhotoView>
  );
}

// Zoomable thumbnail for local files that have not been sent yet.
function LocalFileImage({ file, name }: { file: File; name: string }) {
  const [objectUrl, setObjectUrl] = useState('');

  useEffect(() => {
    const url = URL.createObjectURL(file);
    setObjectUrl(url);
    return () => URL.revokeObjectURL(url);
  }, [file]);

  if (!objectUrl) {
    return null;
  }

  return (
    <PhotoView src={objectUrl}>
      <img
        src={objectUrl}
        alt={name}
        className="size-10 object-cover cursor-zoom-in"
      />
    </PhotoView>
  );
}

type NameWidgetType = {
  name: string;
  size: number;
  id?: string;
};
function NameWidget({ name, size }: NameWidgetType) {
  return (
    <div className="text-xs max-w-20">
      {/* {id ? (
        <NewDocumentLink documentId={id} documentName={name} resource="document">
          {name}
        </NewDocumentLink>
      ) : (
      )} */}
      <div className="truncate">{name}</div>
      <p className="text-text-secondary pt-1">{formatBytes(size)}</p>
    </div>
  );
}
export function InnerUploadedMessageFiles({ files = [] }: IProps) {
  return (
    <PhotoProvider>
      <section className="flex gap-2 pt-2 flex-wrap">
        {files?.map((file, idx) => {
          const name = file.name;
          const isFile = file instanceof File;
          const isImage = isImageFile(file);

          return (
            <div key={idx} className="flex gap-1 border rounded-md p-1.5">
              {isImage ? (
                isFile ? (
                  <LocalFileImage file={file} name={name}></LocalFileImage>
                ) : (
                  <UploadedFileImage
                    id={file.id}
                    name={name}
                    mimeType={getFileMimeType(file)}
                  ></UploadedFileImage>
                )
              ) : !isFile ? (
                <FileIcon id={file.id} name={name}></FileIcon>
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
    </PhotoProvider>
  );
}

export const UploadedMessageFiles = memo(InnerUploadedMessageFiles);
