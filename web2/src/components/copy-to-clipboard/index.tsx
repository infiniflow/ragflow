import { CopyOutlined } from '@ant-design/icons';
import { message } from 'antd';
import { FC } from 'react';

interface CopyToClipboardProps {
  text: string;
}

const CopyToClipboard: FC<CopyToClipboardProps> = ({ text }) => {
  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      message.success('Copied to clipboard');
    } catch (err) {
      message.error('Failed to copy');
    }
  };

  return <CopyOutlined onClick={handleCopy} />;
};

export default CopyToClipboard; 