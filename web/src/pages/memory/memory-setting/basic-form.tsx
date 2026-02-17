import { AvatarUpload } from '@/components/avatar-upload';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Input } from '@/components/ui/input';
import { t } from 'i18next';
import { z } from 'zod';
export const basicInfoSchema = {
  name: z.string().min(1, { message: t('setting.nameRequired') }),
  avatar: z.string().optional(),
  description: z.string().optional(),
};
export const defaultBasicInfo = { name: '', avatar: '', description: '' };
export const BasicInfo = () => {
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
          return <Input {...field}></Input>;
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
          return <AvatarUpload {...field}></AvatarUpload>;
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
          return <Input {...field}></Input>;
        }}
      </RAGFlowFormItem>
    </>
  );
};
