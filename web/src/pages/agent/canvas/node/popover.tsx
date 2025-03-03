import { useFetchFlow } from '@/hooks/flow-hooks';
import get from 'lodash/get';
import React, { MouseEventHandler, useCallback, useMemo } from 'react';
import JsonView from 'react18-json-view';
import 'react18-json-view/src/style.css';
import { useReplaceIdWithText } from '../../hooks';

import { useTheme } from '@/components/theme-provider';
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
import { useGetComponentLabelByValue } from '../../hooks/use-get-begin-query';

interface IProps extends React.PropsWithChildren {
  nodeId: string;
  name?: string;
}

export function NextNodePopover({ children, nodeId, name }: IProps) {
  const { t } = useTranslate('flow');

  const { data } = useFetchFlow();
  const { theme } = useTheme();
  const component = useMemo(() => {
    return get(data, ['dsl', 'components', nodeId], {});
  }, [nodeId, data]);

  const inputs: Array<{ component_id: string; content: string }> = get(
    component,
    ['obj', 'inputs'],
    [],
  );
  const output = get(component, ['obj', 'output'], {});
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
        className="w-[400px]"
      >
        <div className="mb-3 font-semibold text-[16px]">
          {name} {t('operationResults')}
        </div>
        <div className="flex w-full gap-4 flex-col">
          <div className="flex flex-col space-y-1.5">
            <span className="font-semibold text-[14px]">{t('input')}</span>
            <div
              style={
                theme === 'dark'
                  ? {
                      backgroundColor: 'rgba(150, 150, 150, 0.2)',
                    }
                  : {}
              }
              className={`bg-gray-100 p-1 rounded`}
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
          <div className="flex flex-col space-y-1.5">
            <span className="font-semibold text-[14px]">{t('output')}</span>
            <div
              style={
                theme === 'dark'
                  ? {
                      backgroundColor: 'rgba(150, 150, 150, 0.2)',
                    }
                  : {}
              }
              className="bg-gray-100 p-1 rounded"
            >
              <JsonView
                src={replacedOutput}
                displaySize={30}
                className="w-full max-h-[300px] break-words overflow-auto"
              />
            </div>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}
