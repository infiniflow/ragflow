import { LlmModelType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import { useSelectLlmOptionsByModelType } from '@/hooks/llm-hooks';
import { cn } from '@/lib/utils';
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

export function LayoutRecognizeFormField({
  name = 'parser_config.layout_recognize',
  horizontal = true,
}: {
  name?: string;
  horizontal?: boolean;
}) {
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
      name={name}
      render={({ field }) => {
        return (
          <FormItem className={'items-center space-y-0 '}>
            <div
              className={cn('flex', {
                'flex-col ': !horizontal,
                'items-center': horizontal,
              })}
            >
              <FormLabel
                tooltip={t('layoutRecognizeTip')}
                className={cn('text-sm text-muted-foreground whitespace-wrap', {
                  ['w-1/4']: horizontal,
                })}
              >
                {t('layoutRecognize')}
              </FormLabel>
              <div className={horizontal ? 'w-3/4' : 'w-full'}>
                <FormControl>
                  <RAGFlowSelect {...field} options={options}></RAGFlowSelect>
                </FormControl>
              </div>
            </div>
            <div className="flex pt-1">
              <div className={horizontal ? 'w-1/4' : 'w-full'}></div>
              <FormMessage />
            </div>
          </FormItem>
        );
      }}
    />
  );
}
