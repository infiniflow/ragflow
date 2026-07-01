'use client';

import { RAGFlowFormItem } from '@/components/ragflow-form';
import {
  FormControl,
  FormField,
  FormItem,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { useTranslation } from 'react-i18next';

export interface CreateDirectoryFormValues {
  name: string;
  rule: string;
}

export function CreateDirectoryFormFields() {
  const { t } = useTranslation();

  return (
    <div className="space-y-4">
      <RAGFlowFormItem
        name="name"
        label={t('knowledgeDetails.directoryName')}
        required
      >
        <Input
          placeholder={t('knowledgeDetails.directoryNamePlaceholder')}
          autoComplete="off"
        />
      </RAGFlowFormItem>

      <FormField
        name="rule"
        render={({ field }) => (
          <FormItem>
            <FormControl>
              <Textarea
                placeholder={t('knowledgeDetails.directoryRulePlaceholder')}
                className="min-h-[120px]"
                {...field}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </div>
  );
}
