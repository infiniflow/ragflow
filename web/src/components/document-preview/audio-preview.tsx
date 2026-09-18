import message from '@/components/ui/message';
import { Spin } from '@/components/ui/spin';
import request from '@/utils/request';
import classNames from 'classnames';
import { useCallback, useEffect, useState } from 'react';

interface AudioPreviewerProps {
  className?: string;
  url: string;
}

export const AudioPreviewer: React.FC<AudioPreviewerProps> = ({
  className,
  url,
}) => {
  const [audioSrc, setAudioSrc] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);

  const fetchAudio = useCallback(async () => {
    setIsLoading(true);
    const res = await request(url, {
      method: 'GET',
      responseType: 'blob',
      onError: () => {
        message.error('Failed to load audio');
        setIsLoading(false);
      },
    });
    const objectUrl = URL.createObjectURL(res.data);
    setAudioSrc(objectUrl);
    setIsLoading(false);
  }, [url]);

  useEffect(() => {
    if (url) {
      fetchAudio();
    }
  }, [url, fetchAudio]);

  useEffect(() => {
    return () => {
      if (audioSrc) {
        URL.revokeObjectURL(audioSrc);
      }
    };
  }, [audioSrc]);

  return (
    <div
      className={classNames(
        'relative w-full h-full p-4 bg-background-paper border border-border-normal rounded-md audio-previewer',
        className,
      )}
    >
      {isLoading && (
        <div className="absolute inset-0 flex items-center justify-center">
          <Spin />
        </div>
      )}

      {!isLoading && audioSrc && (
        <div className="flex h-full items-center justify-center">
          <audio
            src={audioSrc}
            controls
            className="w-full max-w-2xl"
            data-testid="document-audio-player"
          />
        </div>
      )}
    </div>
  );
};
