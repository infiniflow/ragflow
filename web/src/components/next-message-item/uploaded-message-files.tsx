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
import { getExtension } from '@/utils/document-util';
import { formatBytes } from '@/utils/file-util';
import { memo } from 'react';
import FileIcon from '../file-icon';
import SvgIcon from '../svg-icon';

interface IProps {
  files?: File[] | IDocumentInfo[] | UploadResponseDataType[];
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
    <section className="flex gap-2 pt-2 flex-wrap">
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
