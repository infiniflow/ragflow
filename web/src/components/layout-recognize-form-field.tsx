import { useTranslate } from '@/hooks/common-hooks';
import { useFetchAllAddedModels } from '@/hooks/use-llm-request';
import { cn } from '@/lib/utils';
import { camelCase } from 'lodash';
import { ReactNode, useMemo } from 'react';
import { useFormContext } from 'react-hook-form';
import { MinerUOptionsFormField } from './mineru-options-form-field';
import { buildModelTree } from './model-tree-select';
import { PaddleOCROptionsFormField } from './paddleocr-options-form-field';
import { TreeSelect, TreeSelectNode } from './tree-select';
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
  OpenDataLoader = 'OpenDataLoader',
  TCADPParser = 'TCADP Parser',
}

export function LayoutRecognizeFormField({
  name = 'parser_config.layout_recognize',
  horizontal = true,
  optionsWithoutLLM,
  label,
  showMineruOptions = true,
  showPaddleocrOptions = true,
  testId,
}: {
  name?: string;
  horizontal?: boolean;
  optionsWithoutLLM?: { value: string; label: string }[];
  label?: ReactNode;
  showMineruOptions?: boolean;
  showPaddleocrOptions?: boolean;
  testId?: string;
}) {
  const form = useFormContext();

  const { t } = useTranslate('knowledgeDetails');
  const { data: allAddedModels } = useFetchAllAddedModels();

  const treeData = useMemo(() => {
    const list = optionsWithoutLLM
      ? optionsWithoutLLM
      : [
          ParseDocumentType.DeepDOC,
          ParseDocumentType.PlainText,
          ParseDocumentType.Docling,
          ParseDocumentType.OpenDataLoader,
          ParseDocumentType.TCADPParser,
        ].map((x) => ({
          label: x === ParseDocumentType.PlainText ? t(camelCase(x)) : x,
          value: x,
        }));

    const prependNodes: TreeSelectNode[] = list.map((x) => ({
      id: x.value,
      title: x.label,
    }));

    const modelTree = buildModelTree(
      allAddedModels,
      ['image2text', 'ocr'],
      (node) => (
        <div className="flex justify-between items-center gap-2 w-full">
          <span className="flex items-center gap-1.5 truncate">
            {node.label}
          </span>
          <span className="text-state-error text-sm flex-shrink-0">
            Experimental
          </span>
        </div>
      ),
    );

    return [...prependNodes, ...modelTree];
  }, [allAddedModels, optionsWithoutLLM, t]);

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
                    <TreeSelect
                      {...field}
                      data={treeData}
                      testId={testId}
                      showSearch
                      defaultExpandAll
                      renderSelected={(node) => {
                        if (!node) return null;
                        return node.label ?? node.title;
                      }}
                    />
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
