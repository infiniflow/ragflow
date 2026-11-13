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

'use client';

import {
  FileUpload,
  FileUploadDropzone,
  FileUploadItem,
  FileUploadItemDelete,
  FileUploadItemMetadata,
  FileUploadItemPreview,
  FileUploadItemProgress,
  FileUploadList,
  FileUploadTrigger,
  type FileUploadProps,
} from '@/components/file-upload';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import {
  ArrowUp,
  CircleStop,
  Globe,
  Paperclip,
  Plus,
  Upload,
  X,
} from 'lucide-react';
import * as React from 'react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import {
  setThinkingLevel,
  ThinkingMode,
  useThinkingLevel,
} from '@/hooks/use-thinking-level';
import { SelectWithSearch } from '../originui/select-with-search';
import { AudioButton } from '../ui/audio-button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '../ui/dropdown-menu';
import { Separator } from '../ui/separator';

export type NextMessageInputOnPressEnterParameter = {
  enableThinking?: string;
  enableInternet?: boolean;
  storeHistoryMessages?: boolean;
  omitSessionId?: boolean;
};

interface NextMessageInputProps {
  disabled: boolean;
  value: string;
  sendDisabled: boolean;
  sendLoading: boolean;
  conversationId: string;
  uploadMethod?: string;
  isShared?: boolean;
  showUploadIcon?: boolean;
  isUploading?: boolean;
  onPressEnter({
    enableThinking,
    enableInternet,
  }: NextMessageInputOnPressEnterParameter): void;
  onInputChange: React.ChangeEventHandler<HTMLTextAreaElement>;
  createConversationBeforeUploadDocument?(message: string): Promise<any>;
  stopOutputMessage?(): void;
  onUpload?: NonNullable<FileUploadProps['onUpload']>;
  removeFile?(file: File): void;
  showReasoning?: boolean;
  showInternet?: boolean;
  resize?: 'none' | 'vertical' | 'horizontal' | 'both';
}

export function NextMessageInput({
  isUploading = false,
  value,
  sendDisabled,
  sendLoading,
  disabled,
  showUploadIcon = true,
  resize = 'none',
  onUpload,
  onInputChange,
  stopOutputMessage,
  onPressEnter,
  removeFile,
  showReasoning = false,
  showInternet = false,
}: NextMessageInputProps) {
  const { t } = useTranslation();
  const [files, setFiles] = React.useState<File[]>([]);
  const [audioInputValue, setAudioInputValue] = React.useState<string | null>(
    null,
  );

  const enableThinking = useThinkingLevel();
  const [enableInternet, setEnableInternet] = useState(false);

  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const [isManualMode, setIsManualMode] = useState(false);
  const [isResizing, setIsResizing] = useState(false);
  const dragStartRef = useRef({ startY: 0, startHeight: 0 });

  const handleResizeStart = useCallback(
    (e: React.MouseEvent<HTMLButtonElement>) => {
      e.preventDefault();
      e.stopPropagation();
      const textarea = textareaRef.current;
      if (!textarea) return;
      dragStartRef.current = {
        startY: e.clientY,
        startHeight: textarea.getBoundingClientRect().height,
      };
      setIsManualMode(true);
      setIsResizing(true);
    },
    [],
  );

  useEffect(() => {
    if (!isResizing) return;

    const handleMouseMove = (e: MouseEvent) => {
      const textarea = textareaRef.current;
      if (!textarea) return;
      const { startY, startHeight } = dragStartRef.current;
      const delta = startY - e.clientY;
      const newHeight = Math.max(40, Math.min(startHeight + delta, 400));
      textarea.style.height = `${newHeight}px`;
    };

    const handleMouseUp = () => {
      setIsResizing(false);
    };

    document.body.style.cursor = 'ns-resize';
    document.body.style.userSelect = 'none';
    window.addEventListener('mousemove', handleMouseMove);
    window.addEventListener('mouseup', handleMouseUp);

    return () => {
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
      window.removeEventListener('mousemove', handleMouseMove);
      window.removeEventListener('mouseup', handleMouseUp);
    };
  }, [isResizing]);

  // Level 5 is not a deeper step on the same ladder: the backend hands it to
  // the agentic RAG agent, which runs its own retrieval instead of the
  // pipeline's.
  const thinkingOptions = [
    {
      label: t('chat.thinkingLevelAgentic'),
      value: '5',
      description: t('chat.thinkingLevelAgenticDescription'),
    },
    { label: t('chat.thinkingLevelNone'), value: '0' },
  ];

  const handleThinkingChange = useCallback((value: string) => {
    setThinkingLevel(
      value === ThinkingMode.Agentic
        ? ThinkingMode.Agentic
        : ThinkingMode.Naive,
    );
  }, []);

  const handleInternetSelect = useCallback(() => {
    setEnableInternet(true);
  }, []);

  const handleInternetRemove = useCallback(() => {
    setEnableInternet(false);
  }, []);

  const pressEnter = useCallback(() => {
    onPressEnter({
      enableThinking,
      enableInternet: showInternet ? enableInternet : false,
    });
  }, [onPressEnter, enableThinking, enableInternet, showInternet]);

  useEffect(() => {
    if (audioInputValue !== null) {
      onInputChange({
        target: { value: audioInputValue },
      } as React.ChangeEvent<HTMLTextAreaElement>);

      setTimeout(() => {
        pressEnter();
        setAudioInputValue(null);
      }, 0);
    }
  }, [
    audioInputValue,
    onInputChange,
    onPressEnter,
    enableThinking,
    enableInternet,
    showInternet,
    pressEnter,
  ]);

  const onFileReject = React.useCallback((file: File, message: string) => {
    toast(message, {
      description: `"${file.name.length > 20 ? `${file.name.slice(0, 20)}...` : file.name}" has been rejected`,
    });
  }, []);

  const submit = React.useCallback(() => {
    // Mirror the send button's disabled condition so pressing Enter with an
    // empty message does nothing — previously it cleared the uploaded file
    // list without sending anything.
    if (sendDisabled || isUploading || sendLoading || !value.trim()) return;
    pressEnter();
    setFiles([]);
  }, [sendDisabled, isUploading, sendLoading, value, pressEnter]);

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      submit();
    }
  };

  const onSubmit = React.useCallback(
    (event: React.FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      submit();
    },
    [submit],
  );

  const handleRemoveFile = React.useCallback(
    (file: File) => () => {
      removeFile?.(file);
    },
    [removeFile],
  );

  return (
    <FileUpload
      value={files}
      onValueChange={setFiles}
      onUpload={onUpload}
      onFileReject={onFileReject}
      className="relative w-full items-center"
      disabled={isUploading || disabled}
    >
      <FileUploadDropzone
        tabIndex={-1}
        // Prevents the dropzone from triggering on click
        onClick={(event) => event.preventDefault()}
        className="absolute top-0 left-0 z-0 flex size-full items-center justify-center rounded-none border-none bg-background/50 p-0 opacity-0 backdrop-blur transition-opacity duration-200 ease-out data-[dragging]:z-10 data-[dragging]:opacity-100"
      >
        <div className="flex flex-col items-center gap-1 text-center">
          <div className="flex items-center justify-center rounded-full border p-2.5">
            <Upload className="size-6 text-muted-foreground" />
          </div>
          <p className="font-medium text-sm">Drag & drop files here</p>
          <p className="text-muted-foreground text-xs">
            Upload max 5 files each up to 5MB
          </p>
        </div>
      </FileUploadDropzone>

      <form
        onSubmit={onSubmit}
        className="
          relative flex w-full flex-col gap-2 rounded-[28px]
          border-0.5 border-border-default bg-bg-card p-2 outline-none
          has-[textarea:focus]:outline-accent-primary has-[textarea:focus]:outline-1 has-[textarea:focus]:outline-offset-2
        "
      >
        {resize === 'vertical' && (
          <button
            type="button"
            onMouseDown={handleResizeStart}
            aria-label="Drag to resize height"
            className="absolute -top-0.5 left-1/2 z-10 flex h-1 w-12 -translate-x-1/2 cursor-ns-resize items-center justify-center rounded-full border border-border-default bg-bg-card transition-colors hover:border-accent-primary"
          >
            <span className="h-0.5 w-full rounded-full bg-border-button" />
          </button>
        )}
        <FileUploadList
          orientation="horizontal"
          className="overflow-x-auto px-0 py-1"
        >
          {files.map((file, index) => (
            <FileUploadItem key={index} value={file} className="max-w-52 p-1.5">
              <FileUploadItemPreview className="size-8 [&>svg]:size-5">
                <FileUploadItemProgress variant="fill" />
              </FileUploadItemPreview>
              <FileUploadItemMetadata size="sm" />
              <FileUploadItemDelete asChild>
                <Button
                  variant="secondary"
                  size="icon"
                  className="-top-1 -right-1 absolute size-4 shrink-0 cursor-pointer rounded-full"
                  onClick={handleRemoveFile(file)}
                >
                  <X className="size-2.5" />
                </Button>
              </FileUploadItemDelete>
            </FileUploadItem>
          ))}
        </FileUploadList>
        <div className="flex items-end gap-2">
          {(showUploadIcon || showInternet) && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  type="button"
                  size="icon-lg"
                  variant="transparent"
                  className="rounded-full border-0"
                  disabled={(isUploading || sendLoading) && !showInternet}
                  data-testid="chat-detail-add-menu"
                >
                  <Plus className="size-5" />
                  <span className="sr-only">{t('common.add')}</span>
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent
                side="top"
                align="start"
                sideOffset={8}
                className="min-w-52 rounded-[20px]"
              >
                {showUploadIcon && (
                  <FileUploadTrigger asChild>
                    <DropdownMenuItem
                      justifyBetween={false}
                      disabled={isUploading || sendLoading}
                      data-testid="chat-detail-attach"
                      className="items-center justify-start gap-2 px-3 py-2"
                    >
                      <Paperclip />
                      <span>{t('chat.addPhotosAndFiles')}</span>
                    </DropdownMenuItem>
                  </FileUploadTrigger>
                )}
                {showInternet && (
                  <DropdownMenuItem
                    justifyBetween={false}
                    onClick={handleInternetSelect}
                    data-testid="chat-detail-internet-toggle"
                    className="items-center justify-start gap-2 px-3 py-2"
                  >
                    <Globe className="size-4" />
                    <span>{t('chat.webSearch')}</span>
                  </DropdownMenuItem>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          )}

          {showInternet && enableInternet && (
            <div
              data-testid="chat-detail-internet-active"
              className="flex h-10 min-w-0 max-w-40 shrink items-center gap-1.5 self-start text-accent-primary"
            >
              <Globe className="size-4 shrink-0" />
              <span className="truncate" title={t('chat.webSearch')}>
                {t('chat.webSearch')}
              </span>
              <Button
                type="button"
                variant="transparent"
                size="icon-xs"
                className="size-5 shrink-0 rounded-full border-0 text-accent-primary hover:bg-accent-primary-5 hover:text-accent-primary focus-visible:bg-accent-primary-5 focus-visible:text-accent-primary"
                onClick={handleInternetRemove}
                data-testid="chat-detail-internet-remove"
                aria-label={`${t('common.delete')} ${t('chat.webSearch')}`}
              >
                <X className="size-3" />
              </Button>
            </div>
          )}

          <Textarea
            ref={textareaRef}
            data-testid="chat-textarea"
            value={value}
            onChange={onInputChange}
            placeholder={t('chat.messagePlaceholder')}
            className="
              min-h-10 max-h-[400px] w-full min-w-0 flex-1 px-0 py-2 overflow-auto
              !outline-none !border-transparent !bg-transparent !shadow-none !ring-transparent !ring-offset-transparent
            "
            disabled={isUploading || disabled || sendLoading}
            onKeyDown={handleKeyDown}
            resize={resize === 'vertical' ? 'none' : resize}
            autoSize={isManualMode ? undefined : { minRows: 1, maxRows: 8 }}
          />

          <div className="flex h-10 shrink-0 items-center gap-2">
            {showReasoning && (
              <>
                <SelectWithSearch
                  value={enableThinking}
                  options={thinkingOptions}
                  onChange={handleThinkingChange}
                  triggerClassName="h-9 w-fit border-0 !bg-transparent px-2 text-sm shadow-none hover:!bg-transparent focus-visible:!bg-transparent"
                  testId="chat-detail-thinking-toggle"
                />
                <Separator orientation="vertical" className="h-5" />
              </>
            )}

            {sendLoading ? (
              <Button
                type="button"
                data-testid="chat-stream-status"
                onClick={stopOutputMessage}
                size="icon-lg"
                className="rounded-full"
              >
                <CircleStop className="size-5" />
              </Button>
            ) : (
              <>
                <AudioButton
                  onOk={setAudioInputValue}
                  testId="chat-detail-audio-toggle"
                />

                <Button
                  type="submit"
                  size="icon-lg"
                  className="rounded-full"
                  disabled={
                    sendDisabled || isUploading || sendLoading || !value.trim()
                  }
                  data-testid="chat-detail-send"
                >
                  <ArrowUp className="size-5" />
                  <span className="sr-only">Send message</span>
                </Button>
              </>
            )}
          </div>
        </div>
      </form>
    </FileUpload>
  );
}
