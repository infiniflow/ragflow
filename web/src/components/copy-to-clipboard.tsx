import { useTranslate } from '@/hooks/common-hooks';
import { cn } from '@/lib/utils';
import { LucideCheck, LucideCopy } from 'lucide-react';
import { useState } from 'react';
import { CopyToClipboard as Clipboard } from 'react-copy-to-clipboard';
import { Button, ButtonProps } from './ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from './ui/tooltip';

const CopyToClipboard = ({
  text,
  className,
  ...buttonProps
}: { text: string } & ButtonProps) => {
  const [copied, setCopied] = useState(false);
  const { t } = useTranslate('common');

  const handleCopy = () => {
    setCopied(true);
    setTimeout(() => {
      setCopied(false);
    }, 2000);
  };

  return (
    <Tooltip open={copied ? true : undefined}>
      <Clipboard text={text} onCopy={handleCopy}>
        <TooltipTrigger asChild>
          <Button
            variant="transparent"
            size="icon-sm"
            {...buttonProps}
            className={cn(className, copied && '!text-state-success')}
          >
            {copied ? <LucideCheck /> : <LucideCopy />}
          </Button>
        </TooltipTrigger>
      </Clipboard>
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
