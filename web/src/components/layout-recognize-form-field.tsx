import { LlmModelType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import { useSelectLlmOptionsByModelType } from '@/hooks/use-llm-request';
import { cn } from '@/lib/utils';
import { camelCase } from 'lodash';
import { ReactNode, useMemo } from 'react';
import { useFormContext } from 'react-hook-form';
import { MinerUOptionsFormField } from './mineru-options-form-field';
import { PaddleOCROptionsFormField } from './paddleocr-options-form-field';
import { SelectWithSearch } from './originui/select-with-search';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from './ui/form';

export const enum ParseDocumentType {
  DeepDOC = 'DeepDOC',
  PlainText = 'Plain Text',
  Docling = 'Docling',
  TCADPParser = 'TCADP Parser',
}

export function LayoutRecognizeFormField({
  name = 'parser_config.layout_recognize',
  horizontal = true,
  optionsWithoutLLM,
  label,
  showMineruOptions = true,
  showPaddleocrOptions = true,
}: {
  name?: string;
  horizontal?: boolean;
  optionsWithoutLLM?: { value: string; label: string }[];
  label?: ReactNode;
  showMineruOptions?: boolean;
  showPaddleocrOptions?: boolean;
}) {
  const form = useFormContext();

  const { t } = useTranslate('knowledgeDetails');
  const allOptions = useSelectLlmOptionsByModelType();

  const options = useMemo(() => {
    const list = optionsWithoutLLM
      ? optionsWithoutLLM
      : [
          ParseDocumentType.DeepDOC,
          ParseDocumentType.PlainText,
          ParseDocumentType.Docling,
          ParseDocumentType.TCADPParser,
        ].map((x) => ({
          label: x === ParseDocumentType.PlainText ? t(camelCase(x)) : x,
          value: x,
        }));

    const image2TextList = [
      ...allOptions[LlmModelType.Image2text],
      ...allOptions[LlmModelType.Ocr],
    ].map((x) => {
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
  }, [allOptions, optionsWithoutLLM, t]);

  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => {
        return (
          <>
            <FormItem className={'items-center space-y-0 '}>
              <div
                className={cn('flex', {
                  'flex-col ': !horizontal,
                  'items-center': horizontal,
                })}
              >
                <FormLabel
                  tooltip={t('layoutRecognizeTip')}
                  className={cn('text-sm text-text-secondary whitespace-wrap', {
                    ['w-1/4']: horizontal,
                  })}
                >
                  {label || t('layoutRecognize')}
                </FormLabel>
                <div className={horizontal ? 'w-3/4' : 'w-full'}>
                  <FormControl>
                    <SelectWithSearch
                      {...field}
                      options={options}
                    ></SelectWithSearch>
                  </FormControl>
                </div>
              </div>
              <div className="flex pt-1">
                <div className={horizontal ? 'w-1/4' : 'w-full'}></div>
                <FormMessage />
              </div>
            </FormItem>
            {showMineruOptions && <MinerUOptionsFormField />}
            {showPaddleocrOptions && <PaddleOCROptionsFormField />}
          </>
        );
      }}
    />
  );
}
