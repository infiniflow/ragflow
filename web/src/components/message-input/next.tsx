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
import { t } from 'i18next';
import {
  Atom,
  CircleStop,
  Globe,
  Paperclip,
  Send,
  Upload,
  X,
} from 'lucide-react';
import * as React from 'react';
import { useCallback, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { AudioButton } from '../ui/audio-button';

export type NextMessageInputOnPressEnterParameter = {
  enableThinking: boolean;
  enableInternet: boolean;
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
  const [files, setFiles] = React.useState<File[]>([]);
  const [audioInputValue, setAudioInputValue] = React.useState<string | null>(
    null,
  );

  const [enableThinking, setEnableThinking] = useState(false);
  const [enableInternet, setEnableInternet] = useState(false);

  const handleThinkingToggle = useCallback(() => {
    setEnableThinking((prev) => !prev);
  }, []);

  const handleInternetToggle = useCallback(() => {
    setEnableInternet((prev) => !prev);
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
    if (isUploading) return;
    pressEnter();
    setFiles([]);
  }, [isUploading, pressEnter]);

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
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
          relative flex w-full flex-col gap-2.5 rounded-md
          border-0.5 border-border-default bg-bg-card p-2 outline-none
          has-[textarea:focus]:outline-accent-primary has-[textarea:focus]:outline-1 has-[textarea:focus]:outline-offset-2
        "
      >
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

        <Textarea
          value={value}
          onChange={onInputChange}
          placeholder={t('chat.messagePlaceholder')}
          className="
            min-h-10 max-h-40 w-full p-0 overflow-auto
            !outline-none !border-transparent !bg-transparent !shadow-none !ring-transparent !ring-offset-transparent
          "
          disabled={isUploading || disabled || sendLoading}
          onKeyDown={handleKeyDown}
          autoSize={{ minRows: 2, maxRows: 8 }}
        />

        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            {showUploadIcon && (
              <FileUploadTrigger asChild>
                <Button
                  type="button"
                  size="icon-xs"
                  variant="transparent"
                  className="rounded-sm border-0"
                  disabled={isUploading || sendLoading}
                >
                  <Paperclip className="size-3.5" />
                  <span className="sr-only">Attach file</span>
                </Button>
              </FileUploadTrigger>
            )}

            {showReasoning && (
              <Button
                type="button"
                size="sm"
                variant={enableThinking ? 'accent' : 'transparent'}
                className="border-0 h-7 text-sm"
                onClick={handleThinkingToggle}
              >
                <Atom />
                <span>Thinking</span>
              </Button>
            )}

            {showInternet && (
              <Button
                type="button"
                variant={enableInternet ? 'accent' : 'transparent'}
                size="icon-xs"
                className="border-0"
                onClick={handleInternetToggle}
              >
                <Globe />
              </Button>
            )}
          </div>

          {sendLoading ? (
            <Button onClick={stopOutputMessage} size="icon-xs">
              <CircleStop />
            </Button>
          ) : (
            <div className="flex items-center gap-3">
              <AudioButton
                onOk={(value) => {
                  setAudioInputValue(value);
                }}
              />

              <Button
                size="icon-xs"
                disabled={
                  sendDisabled || isUploading || sendLoading || !value.trim()
                }
              >
                <Send />
                <span className="sr-only">Send message</span>
              </Button>
            </div>
          )}
        </div>
      </form>
    </FileUpload>
  );
}
