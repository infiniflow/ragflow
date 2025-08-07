import { ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useSetAgentSetting } from '@/hooks/use-agent-request';
import { IModalProps } from '@/interfaces/common';
import { transformFile2Base64 } from '@/utils/file-util';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  AgentSettingId,
  SettingForm,
  SettingFormSchemaType,
} from './setting-form';

export function SettingDialog({ hideModal }: IModalProps<any>) {
  const { t } = useTranslation();
  const { setAgentSetting } = useSetAgentSetting();

  const submit = useCallback(
    async (values: SettingFormSchemaType) => {
      const avatar = values.avatar;
      const code = await setAgentSetting({
        ...values,
        avatar: avatar.length > 0 ? await transformFile2Base64(avatar[0]) : '',
      });
      if (code === 0) {
        hideModal?.();
      }
    },
    [hideModal, setAgentSetting],
  );

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
