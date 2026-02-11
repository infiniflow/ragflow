import { useTranslate } from '@/hooks/common-hooks';
import { CheckOutlined, CopyOutlined } from '@ant-design/icons';
import { useState } from 'react';
import { CopyToClipboard as Clipboard, Props } from 'react-copy-to-clipboard';
import { Tooltip, TooltipContent, TooltipTrigger } from './ui/tooltip';

const CopyToClipboard = ({ text }: Props) => {
  const [copied, setCopied] = useState(false);
  const { t } = useTranslate('common');

  const handleCopy = () => {
    setCopied(true);
    setTimeout(() => {
      setCopied(false);
    }, 2000);
  };

  return (
    <Tooltip>
      <TooltipTrigger>
        <Clipboard text={text} onCopy={handleCopy}>
          {copied ? <CheckOutlined /> : <CopyOutlined />}
        </Clipboard>
      </TooltipTrigger>
      <TooltipContent>{copied ? t('copied') : t('copy')}</TooltipContent>
    </Tooltip>
  );
};

export default CopyToClipboard;

export function CopyToClipboardWithText({ text }: { text: string }) {
  return (
    <div className="bg-bg-card p-1 rounded-md flex gap-2">
      <span className="flex-1 truncate">{text}</span>
      <CopyToClipboard text={text}></CopyToClipboard>
    </div>
  );
}
