import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Input } from '@/components/ui/input';
import { isEmpty, isNil } from 'lodash';
import { useEffect } from 'react';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { DynamicPageRange } from './dynamic-page-range';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix } from './utils';

export function PagingFormFields({ prefix }: CommonProps) {
  const { t } = useTranslation();
  const form = useFormContext();

  const taskPageSizeName = buildFieldNameWithPrefix('task_page_size', prefix);
  const pagesName = buildFieldNameWithPrefix('pages', prefix);

  useEffect(() => {
    if (isNil(form.getValues(taskPageSizeName))) {
      form.setValue(taskPageSizeName, 12, {
        shouldValidate: true,
        shouldDirty: true,
      });
    }
  }, [form, taskPageSizeName]);

  useEffect(() => {
    if (isEmpty(form.getValues(pagesName))) {
      form.setValue(pagesName, [{ from: 1, to: 100000 }], {
        shouldValidate: true,
        shouldDirty: true,
      });
    }
  }, [form, pagesName]);

  return (
    <>
      <RAGFlowFormItem
        name={taskPageSizeName}
        label={t('knowledgeDetails.taskPageSize')}
        tooltip={t('knowledgeDetails.taskPageSizeTip')}
      >
        <Input type={'number'} min={1} max={128} />
      </RAGFlowFormItem>
      <DynamicPageRange prefix={prefix} />
    </>
  );
}
