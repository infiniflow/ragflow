import { useFetchFlow } from '@/hooks/flow-hooks';
import get from 'lodash/get';
import React, { MouseEventHandler, useCallback, useMemo } from 'react';
import JsonView from 'react18-json-view';
import 'react18-json-view/src/style.css';
import { useReplaceIdWithText } from '../../hooks';

import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { useTranslate } from '@/hooks/common-hooks';
import { Input } from 'antd';
import { useGetComponentLabelByValue } from '../../hooks/use-get-begin-query';

interface IProps extends React.PropsWithChildren {
  nodeId: string;
  name?: string;
}

export function NextNodePopover({ children, nodeId, name }: IProps) {
  const { t } = useTranslate('flow');

  const { data } = useFetchFlow();
  console.log(data);

  const component = useMemo(() => {
    return get(data, ['dsl', 'components', nodeId], {});
  }, [nodeId, data]);

  const inputs: Array<{ component_id: string; content: string }> = get(
    component,
    ['obj', 'inputs'],
    [],
  );
  const output = get(component, ['obj', 'output'], {});
  const { conf, messages, prompt } = get(
    component,
    ['obj', 'params', 'infor'],
    {},
  );
  const { replacedOutput } = useReplaceIdWithText(output);
  const stopPropagation: MouseEventHandler = useCallback((e) => {
    e.stopPropagation();
  }, []);

  const getLabel = useGetComponentLabelByValue(nodeId);

  return (
    <Popover>
      <PopoverTrigger onClick={stopPropagation} asChild>
        {children}
      </PopoverTrigger>
      <PopoverContent
        align={'start'}
        side={'right'}
        sideOffset={20}
        onClick={stopPropagation}
        className="w-[800px] p-4"
      >
        <div className="mb-4 font-semibold text-[18px] border-b pb-2">
          {name} {t('operationResults')}
        </div>
        <div className="flex w-full gap-5 flex-col">
          <div className="flex flex-col space-y-2">
            <span className="font-semibold text-[15px] text-gray-700 dark:text-gray-300">
              {t('input')}
            </span>
            <div
              className={`bg-gray-50 dark:bg-gray-800 p-3 rounded-lg border dark:border-gray-700`}
            >
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('componentId')}</TableHead>
                    <TableHead className="w-[60px]">{t('content')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {inputs.map((x, idx) => (
                    <TableRow key={idx}>
                      <TableCell>{getLabel(x.component_id)}</TableCell>
                      <TableCell className="truncate">{x.content}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </div>
          <div className="flex flex-col space-y-2">
            <span className="font-semibold text-[15px] text-gray-700 dark:text-gray-300">
              {t('output')}
            </span>
            <div className="bg-gray-50 dark:bg-gray-800 p-3 rounded-lg border dark:border-gray-700">
              <JsonView
                src={replacedOutput}
                displaySize={30}
                className="w-full max-h-[300px] break-words overflow-auto"
              />
            </div>
          </div>
          <div className="flex flex-col space-y-2">
            <span className="font-semibold text-[15px] text-gray-700 dark:text-gray-300">
              {t('infor')}
            </span>
            <div className="bg-gray-50 dark:bg-gray-800 p-3 rounded-lg border dark:border-gray-700">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  {conf && (
                    <div className="mb-4">
                      <div className="font-medium mb-2 text-gray-600 dark:text-gray-400">
                        Configuration:
                      </div>
                      <JsonView
                        src={conf}
                        displaySize={30}
                        className="w-full max-h-[120px] break-words overflow-auto"
                      />
                    </div>
                  )}
                  {prompt && (
                    <div>
                      <div className="font-medium mb-2 text-gray-600 dark:text-gray-400">
                        Prompt:
                      </div>
                      <Input.TextArea
                        value={prompt}
                        readOnly
                        autoSize={{ minRows: 2, maxRows: 6 }}
                        className="bg-white dark:bg-gray-900 border-gray-200 dark:border-gray-700"
                      />
                    </div>
                  )}
                </div>
                <div>
                  {messages && (
                    <div className="mb-4">
                      <div className="font-medium mb-2 text-gray-600 dark:text-gray-400">
                        Messages:
                      </div>
                      <div className="max-h-[300px] overflow-auto">
                        <JsonView
                          src={messages}
                          displaySize={30}
                          className="w-full break-words"
                        />
                      </div>
                    </div>
                  )}
                </div>
              </div>
            </div>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}
