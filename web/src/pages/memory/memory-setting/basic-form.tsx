import { AvatarUpload } from '@/components/avatar-upload';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Input } from '@/components/ui/input';
import { DESCRIPTION_MAX_LENGTH } from '@/constants/common';
import { t } from 'i18next';
import { z } from 'zod';
export const basicInfoSchema = {
  name: z.string().min(1, { message: t('setting.nameRequired') }),
  avatar: z.string().optional(),
  description: z
    .string()
    .max(DESCRIPTION_MAX_LENGTH, {
      message: t('common.descriptionMaxLength', {
        max: DESCRIPTION_MAX_LENGTH,
      }),
    })
    .optional(),
};
export const defaultBasicInfo = { name: '', avatar: '', description: '' };
export const BasicInfo = ({ readOnly = false }: { readOnly?: boolean }) => {
  return (
    <>
      <RAGFlowFormItem
        name={'name'}
        label={t('memories.name')}
        required={true}
        horizontal={true}
        // tooltip={field.tooltip}
        // labelClassName={labelClassName || field.labelClassName}
      >
        {(field) => {
          return <Input {...field} disabled={readOnly}></Input>;
        }}
      </RAGFlowFormItem>
      <RAGFlowFormItem
        name={'avatar'}
        label={t('memory.config.avatar')}
        required={false}
        horizontal={true}
        // tooltip={field.tooltip}
        // labelClassName={labelClassName || field.labelClassName}
      >
        {(field) => {
          return (
            <div className={readOnly ? 'pointer-events-none opacity-60' : ''}>
              <AvatarUpload {...field}></AvatarUpload>
            </div>
          );
        }}
      </RAGFlowFormItem>
      <RAGFlowFormItem
        name={'description'}
        label={t('memory.config.description')}
        required={false}
        horizontal={true}
        className="!items-start"
        // tooltip={field.tooltip}
        // labelClassName={labelClassName || field.labelClassName}
      >
        {(field) => {
          return (
            <Input
              {...field}
              disabled={readOnly}
              maxLength={DESCRIPTION_MAX_LENGTH}
              placeholder={t('memory.config.descriptionPlaceholder')}
            ></Input>
          );
        }}
      </RAGFlowFormItem>
    </>
  );
};
