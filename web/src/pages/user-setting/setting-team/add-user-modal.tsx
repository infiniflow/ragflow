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

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Modal } from '@/components/ui/modal/modal';
import { IModalProps } from '@/interfaces/common';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import * as z from 'zod';

const AddingUserModal = ({
  visible,
  hideModal,
  loading,
  onOk,
}: IModalProps<string>) => {
  const { t } = useTranslation();

  const formSchema = z.object({
    email: z
      .string()
      .email()
      .min(1, { message: t('common.required') }),
  });

  type FormData = z.infer<typeof formSchema>;

  const form = useForm<FormData>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      email: '',
    },
  });

  const handleOk = async (data: FormData) => {
    return onOk?.(data.email);
  };

  return (
    <Modal
      title={t('setting.add')}
      open={visible || false}
      onOpenChange={(open) => !open && hideModal?.()}
      onOk={form.handleSubmit(handleOk)}
      confirmLoading={loading}
      okText={t('common.ok')}
      cancelText={t('common.cancel')}
    >
      <Form {...form}>
        <form onSubmit={form.handleSubmit(handleOk)} className="space-y-4">
          <FormField
            control={form.control}
            name="email"
            render={({ field }) => (
              <FormItem>
                <FormLabel required>{t('setting.email')}</FormLabel>
                <FormControl>
                  <Input placeholder={t('setting.email')} {...field} />
                </FormControl>
                <FormDescription className="text-xs">
                  {t('setting.inviteTip')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </form>
      </Form>
    </Modal>
  );
};

export default AddingUserModal;
