import { Spin } from '@/components/ui/spin';
import request from '@/utils/request';
import classNames from 'classnames';
import { useEffect, useState } from 'react';

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
  const [loadFailed, setLoadFailed] = useState<boolean>(false);

  useEffect(() => {
    if (!url) {
      return;
    }
    let stale = false;
    setIsLoading(true);
    setLoadFailed(false);

    const loadAudio = async () => {
      try {
        const res = await request(url, {
          method: 'GET',
          responseType: 'blob',
        });
        if (stale) {
          return;
        }
        if (!(res.data instanceof Blob)) {
          setLoadFailed(true);
          return;
        }
        setAudioSrc(URL.createObjectURL(res.data));
      } catch {
        if (!stale) {
          setLoadFailed(true);
        }
      } finally {
        if (!stale) {
          setIsLoading(false);
        }
      }
    };

    loadAudio();

    return () => {
      stale = true;
    };
  }, [url]);

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

      {!isLoading && !audioSrc && loadFailed && (
        <div
          className="flex h-full items-center justify-center text-text-secondary"
          data-testid="document-audio-error"
        >
          Failed to load audio
        </div>
      )}
    </div>
  );
};
