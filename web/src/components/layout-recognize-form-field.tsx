import { LlmModelType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import { useSelectLlmOptionsByModelType } from '@/hooks/llm-hooks';
import { camelCase } from 'lodash';
import { useMemo } from 'react';
import { useFormContext } from 'react-hook-form';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from './ui/form';
import { RAGFlowSelect } from './ui/select';

export const enum DocumentType {
  DeepDOC = 'DeepDOC',
  PlainText = 'Plain Text',
}

export function LayoutRecognizeFormField() {
  const form = useFormContext();

  const { t } = useTranslate('knowledgeDetails');
  const allOptions = useSelectLlmOptionsByModelType();

  const options = useMemo(() => {
    const list = [DocumentType.DeepDOC, DocumentType.PlainText].map((x) => ({
      label: x === DocumentType.PlainText ? t(camelCase(x)) : 'DeepDoc',
      value: x,
    }));

    const image2TextList = allOptions[LlmModelType.Image2text].map((x) => {
      return {
        ...x,
        options: x.options.map((y) => {
          return {
            ...y,
            label: (
              <div className="flex justify-between items-center gap-2">
                {y.label}
                <span className="text-red-500 text-sm">Experimental</span>
              </div>
            ),
          };
        }),
      };
    });

    return [...list, ...image2TextList];
  }, [allOptions, t]);

  return (
    <FormField
      control={form.control}
      name="parser_config.layout_recognize"
      render={({ field }) => (
        <FormItem>
          <FormLabel tooltip={t('layoutRecognizeTip')}>
            {t('layoutRecognize')}
          </FormLabel>
          <FormControl>
            <RAGFlowSelect {...field} options={options}></RAGFlowSelect>
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
