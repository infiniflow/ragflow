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

import { memo } from 'react';

import { Images } from '@/constants/common';
import CSVFileViewer from './csv-preview';
import { DocPreviewer } from './doc-preview';
import { EpubPreviewer } from './epub-preview';
import { ExcelCsvPreviewer } from './excel-preview';
import { ImagePreviewer } from './image-preview';
import { Md } from './md';
import PdfPreviewer, { IProps } from './pdf-preview';
import { PptPreviewer } from './ppt-preview';
import { TxtPreviewer } from './txt-preview';
import { VideoPreviewer } from './video-preview';

type PreviewProps = {
  fileType: string;
  className?: string;
  url: string;
  positions?: number[][];
};
const DocumentPreview = function ({
  fileType,
  className,
  highlights,
  setWidthAndHeight,
  url,
  positions,
}: PreviewProps & Partial<IProps>) {
  const isPdf = fileType === 'pdf';

  return (
    <>
      {isPdf && (
        <section className="h-full">
          <PdfPreviewer
            className={className}
            highlights={highlights}
            setWidthAndHeight={setWidthAndHeight}
            url={url}
          ></PdfPreviewer>
        </section>
      )}
      {['doc', 'docx'].indexOf(fileType) > -1 && (
        <section>
          <DocPreviewer className={className} url={url} />
        </section>
      )}
      {['txt', 'json'].indexOf(fileType) > -1 && (
        <section>
          <TxtPreviewer className={className} url={url} />
        </section>
      )}
      {Images.indexOf(fileType) > -1 && (
        <section>
          <ImagePreviewer className={className} url={url} />
        </section>
      )}
      {[
        'mp4',
        'avi',
        'mov',
        'mkv',
        'wmv',
        'flv',
        'mpeg',
        'mpg',
        'asf',
        'rm',
        'rmvb',
      ].indexOf(fileType) > -1 && (
        <section>
          <VideoPreviewer className={className} url={url} />
        </section>
      )}
      {['ppt', 'pptx'].indexOf(fileType) > -1 && (
        <section>
          <PptPreviewer className={className} url={url} />
        </section>
      )}
      {['xlsx', 'xls'].indexOf(fileType) > -1 && (
        <section className="h-full">
          <ExcelCsvPreviewer
            className={className}
            url={url}
            positions={positions}
          />
        </section>
      )}
      {['csv'].indexOf(fileType) > -1 && (
        <section>
          <CSVFileViewer className={className} url={url} />
        </section>
      )}
      {['md', 'mdx'].indexOf(fileType) > -1 && (
        <section>
          <Md className={className} url={url} />
        </section>
      )}
      {['epub'].indexOf(fileType) > -1 && (
        <section>
          <EpubPreviewer className={className} url={url} />
        </section>
      )}
    </>
  );
};
export default memo(DocumentPreview);
