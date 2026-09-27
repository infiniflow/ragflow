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
import classNames from 'classnames';
import { useCallback, useEffect, useState } from 'react';

interface VideoPreviewerProps {
  className?: string;
  url: string;
}

export const VideoPreviewer: React.FC<VideoPreviewerProps> = ({
  className,
  url,
}) => {
  // const url = useGetDocumentUrl();
  const [videoSrc, setVideoSrc] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);

  const fetchVideo = useCallback(async () => {
    setIsLoading(true);
    const res = await request(url, {
      method: 'GET',
      responseType: 'blob',
      onError: () => {
        message.error('Failed to load video');
        setIsLoading(false);
      },
    });
    const objectUrl = URL.createObjectURL(res.data);
    setVideoSrc(objectUrl);
    setIsLoading(false);
  }, [url]);

  useEffect(() => {
    if (url) {
      fetchVideo();
    }
  }, [url, fetchVideo]);

  useEffect(() => {
    return () => {
      if (videoSrc) {
        URL.revokeObjectURL(videoSrc);
      }
    };
  }, [videoSrc]);

  return (
    <div
      className={classNames(
        'relative w-full h-full p-4 bg-background-paper border border-border-normal rounded-md video-previewer',
        className,
      )}
    >
      {isLoading && (
        <div className="absolute inset-0 flex items-center justify-center">
          <Spin />
        </div>
      )}

      {!isLoading && videoSrc && (
        <div className="max-h-[80vh] overflow-auto p-2">
          <video
            src={videoSrc}
            controls
            className="w-full h-auto max-w-full object-contain"
            onLoadedData={() => URL.revokeObjectURL(videoSrc!)}
          />
        </div>
      )}
    </div>
  );
};
