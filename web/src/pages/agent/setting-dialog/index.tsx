import { ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { IModalProps } from '@/interfaces/common';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  AgentSettingId,
  SettingForm,
  SettingFormSchemaType,
} from './setting-form';

export function SettingDialog({ hideModal }: IModalProps<any>) {
  const { t } = useTranslation();

  const submit = useCallback((values: SettingFormSchemaType) => {
    console.log('🚀 ~ SettingDialog ~ values:', values);
  }, []);

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Are you absolutely sure?</DialogTitle>
        </DialogHeader>
        <SettingForm submit={submit}></SettingForm>
        <DialogFooter>
          <ButtonLoading type="submit" form={AgentSettingId} loading={false}>
            {t('common.save')}
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
