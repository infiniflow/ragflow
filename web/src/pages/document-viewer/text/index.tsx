import React, { useEffect, useState } from 'react';
import FileError from '../file-error';

interface TxtProps {
  filePath: string;
}

const Md: React.FC<TxtProps> = ({ filePath }) => {
  const [content, setContent] = useState<string>('');
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setError(null);
    fetch(filePath)
      .then((res) => {
        if (!res.ok) throw new Error('Failed to fetch text file');
        return res.text();
      })
      .then((text) => setContent(text))
      .catch((err) => setError(err.message));
  }, [filePath]);

  if (error) return <FileError>{error}</FileError>;

  return (
    <div style={{ padding: 24, height: '100vh', overflow: 'scroll' }}>
      {content}
    </div>
  );
};

export default Md;
