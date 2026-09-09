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
  avoidButtonWrapper = false,
  ...buttonProps
}: { text: string; avoidButtonWrapper?: boolean } & ButtonProps) => {
  const [copied, setCopied] = useState(false);
  const { t } = useTranslate('common');

  const handleCopy = () => {
    setCopied(true);
    setTimeout(() => {
      setCopied(false);
    }, 2000);
  };

  const icon = copied ? <LucideCheck /> : <LucideCopy />;
  const trigger = avoidButtonWrapper ? (
    <Button
      variant="transparent"
      size="icon-sm"
      {...buttonProps}
      className={cn(className, copied && '!text-state-success')}
    >
      <span aria-label={copied ? t('copied') : t('copy')}>{icon}</span>
    </Button>
  ) : (
    <Button
      variant="transparent"
      size="icon-sm"
      {...buttonProps}
      className={cn(className, copied && '!text-state-success')}
    >
      {icon}
    </Button>
  );

  return (
    <Tooltip open={copied ? true : undefined}>
      <Clipboard text={text} onCopy={handleCopy}>
        <TooltipTrigger asChild>{trigger}</TooltipTrigger>
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
