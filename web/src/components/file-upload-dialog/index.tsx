import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { IModalProps } from '@/interfaces/common';
import { Dispatch, SetStateAction, useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FileUploader } from '../file-uploader';

type UploaderTabsProps = {
  setFiles: Dispatch<SetStateAction<File[]>>;
};

export function UploaderTabs({ setFiles }: UploaderTabsProps) {
  const { t } = useTranslation();

  return (
    <Tabs defaultValue="account">
      <TabsList className="grid w-full grid-cols-2 mb-4">
        <TabsTrigger value="account">{t('fileManager.local')}</TabsTrigger>
        <TabsTrigger value="password">{t('fileManager.s3')}</TabsTrigger>
      </TabsList>
      <TabsContent value="account">
        <FileUploader
          maxFileCount={8}
          maxSize={8 * 1024 * 1024}
          onValueChange={setFiles}
        />
      </TabsContent>
      <TabsContent value="password">{t('common.comingSoon')}</TabsContent>
    </Tabs>
  );
}

export function FileUploadDialog({ hideModal, onOk }: IModalProps<File[]>) {
  const { t } = useTranslation();
  const [files, setFiles] = useState<File[]>([]);

  const handleOk = useCallback(() => {
    onOk?.(files);
  }, [files, onOk]);

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>{t('fileManager.uploadFile')}</DialogTitle>
        </DialogHeader>
        <UploaderTabs setFiles={setFiles}></UploaderTabs>
        <DialogFooter>
          <Button
            type="submit"
            variant={'tertiary'}
            size={'sm'}
            onClick={handleOk}
          >
            {t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
