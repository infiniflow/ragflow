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

import {
  getExtension,
  isSupportedPreviewDocumentType,
} from '@/utils/document-util';
import React from 'react';

interface IProps extends React.PropsWithChildren {
  link?: string;
  preventDefault?: boolean;
  color?: string;
  documentName: string;
  documentId?: string;
  resource?: 'document' | 'files';
  className?: string;
}

const NewDocumentLink = ({
  children,
  link,
  preventDefault = false,
  color = 'rgb(15, 79, 170)',
  documentId,
  documentName,
  resource = 'document',
  className,
}: IProps) => {
  let nextLink = link;
  const extension = getExtension(documentName);
  if (!link) {
    nextLink = `/document/${documentId}?ext=${extension}&resource=${resource}`;
  }

  return (
    <a
      target="_blank"
      onClick={
        !preventDefault || isSupportedPreviewDocumentType(extension)
          ? undefined
          : (e) => e.preventDefault()
      }
      href={nextLink}
      rel="noreferrer"
      style={{ color: className ? '' : color, wordBreak: 'break-all' }}
      className={className}
    >
      {children}
    </a>
  );
};

export default NewDocumentLink;
