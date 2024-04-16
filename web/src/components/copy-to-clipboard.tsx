import { CheckOutlined, CopyOutlined } from '@ant-design/icons';
import { Tooltip } from 'antd';
import { useState } from 'react';
import { CopyToClipboard as Clipboard, Props } from 'react-copy-to-clipboard';

const CopyToClipboard = ({ children, text }: Props) => {
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    setCopied(true);
    setTimeout(() => {
      setCopied(false);
    }, 2000);
  };

  return (
    <Tooltip title={copied ? '复制成功' : '复制'}>
      <Clipboard text={text} onCopy={handleCopy}>
        {copied ? <CheckOutlined /> : <CopyOutlined />}
      </Clipboard>
    </Tooltip>
  );
};

export default CopyToClipboard;
