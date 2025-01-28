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
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FileUploader } from '../file-uploader';

export function UploaderTabs() {
  const { t } = useTranslation();

  const [files, setFiles] = useState<File[]>([]);
  console.log('🚀 ~ TabsDemo ~ files:', files);

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

export function FileUploadDialog({ hideModal }: IModalProps<any>) {
  const { t } = useTranslation();

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>{t('fileManager.uploadFile')}</DialogTitle>
        </DialogHeader>
        <UploaderTabs></UploaderTabs>
        <DialogFooter>
          <Button type="submit" variant={'tertiary'} size={'sm'}>
            {t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
