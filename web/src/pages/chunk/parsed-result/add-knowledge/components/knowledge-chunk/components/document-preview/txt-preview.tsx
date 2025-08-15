import message from '@/components/ui/message';
import { Spin } from '@/components/ui/spin';
import request from '@/utils/request';
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
    // blob to string
    const reader = new FileReader();
    reader.readAsText(res.data);
    reader.onload = () => {
      setData(reader.result as string);
      setLoading(false);
      console.log('file loaded successfully', reader.result);
    };
    console.log('file data:', res);
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
        'relative w-full h-full p-4 bg-background-paper border border-border-normal rounded-md',
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
