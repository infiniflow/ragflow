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

import message from '@/components/ui/message';
import { Spin } from '@/components/ui/spin';
import request from '@/utils/request';
import { decodeBlobText } from '@/utils/file-util';
import classNames from 'classnames';
import { useEffect, useState } from 'react';

type TxtPreviewerProps = { className?: string; url: string };
export const TxtPreviewer = ({ className, url }: TxtPreviewerProps) => {
  // const url = useGetDocumentUrl();
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState<string>('');
  const fetchTxt = async () => {
    setLoading(true);
    const res = await request(url, {
      method: 'GET',
      responseType: 'blob',
      onError: (err: any) => {
        message.error('Failed to load file');
        console.error('Error loading file:', err);
      },
    });
    // Handles UTF-8/UTF-16 (BOM) as well as GB2312/GBK files
    const text = await decodeBlobText(res.data);
    setData(text);
    setLoading(false);
  };
  useEffect(() => {
    if (url) {
      fetchTxt();
    } else {
      setLoading(false);
      setData('');
    }
  }, [url]);
  return (
    <div
      className={classNames(
        'relative w-full h-full p-4 overflow-auto bg-background-paper border border-border-normal rounded-md',
        className,
      )}
    >
      {loading && (
        <div className="absolute inset-0 flex items-center justify-center">
          <Spin />
        </div>
      )}

      {!loading && <pre className="whitespace-pre-wrap p-2 ">{data}</pre>}
    </div>
  );
};
