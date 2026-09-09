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

import { getExtension } from '@/utils/document-util';
import { File as LucideFile } from 'lucide-react';
import SvgIcon, { hasSvgIcon } from '../svg-icon';

import { useFetchDocumentThumbnailsByIds } from '@/hooks/use-document-request';
import { useAuthenticatedImageUrl } from '@/components/image';
import { useEffect } from 'react';
import styles from './index.module.less';

interface IProps {
  name: string;
  id: string;
}

const FileIcon = ({ name, id }: IProps) => {
  const fileExtension = getExtension(name);

  const { data: fileThumbnails, setDocumentIds } =
    useFetchDocumentThumbnailsByIds();
  const fileThumbnail = fileThumbnails[id];
  const blobUrl = useAuthenticatedImageUrl(fileThumbnail);

  useEffect(() => {
    if (id) {
      setDocumentIds([id]);
    }
  }, [id, setDocumentIds]);

  const iconName = `file-icon/${fileExtension}`;

  return blobUrl ? (
    <img src={blobUrl} className={styles.thumbnailImg}></img>
  ) : hasSvgIcon(iconName) ? (
    <SvgIcon name={iconName} width={24}></SvgIcon>
  ) : (
    <LucideFile size={24}></LucideFile>
  );
};

export default FileIcon;
