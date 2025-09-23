import { useTranslate } from '@/hooks/common-hooks';
import { useFetchAgentList } from '@/hooks/use-agent-request';
import { buildSelectOptions } from '@/utils/component-util';
import { ArrowUpRight } from 'lucide-react';
import { useMemo } from 'react';
import { useFormContext } from 'react-hook-form';
import { SelectWithSearch } from '../originui/select-with-search';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '../ui/form';
import { MultiSelect } from '../ui/multi-select';

interface IProps {
  toDataPipeline?: () => void;
  formFieldName: string;
  isMult?: boolean;
}

export function DataFlowSelect(props: IProps) {
  const { toDataPipeline, formFieldName, isMult = true } = props;
  const { t } = useTranslate('knowledgeConfiguration');
  const form = useFormContext();
  const toDataPipLine = () => {
    toDataPipeline?.();
  };
  const { data: dataPipelineOptions, loading } = useFetchAgentList({
    canvas_category: 'dataflow_canvas',
  });
  const options = useMemo(() => {
    const option = buildSelectOptions(
      dataPipelineOptions?.canvas,
      'id',
      'title',
    );
    return option || [];
  }, [dataPipelineOptions]);
  return (
    <FormField
      control={form.control}
      name={formFieldName}
      render={({ field }) => (
        <FormItem className=" items-center space-y-0 ">
          <div className="flex flex-col gap-1">
            <div className="flex gap-2 justify-between ">
              <FormLabel
                tooltip={t('dataFlowTip')}
                className="text-sm text-text-primary whitespace-wrap "
              >
                {t('dataFlow')}
              </FormLabel>
              {toDataPipeline && (
                <div
                  className="text-sm flex text-text-primary cursor-pointer"
                  onClick={toDataPipLine}
                >
                  {t('buildItFromScratch')}
                  <ArrowUpRight size={14} />
                </div>
              )}
            </div>

            <div className="text-muted-foreground">
              <FormControl>
                <>
                  {!isMult && (
                    <SelectWithSearch
                      {...field}
                      placeholder={t('dataFlowPlaceholder')}
                      options={options}
                    />
                  )}
                  {isMult && (
                    <MultiSelect
                      {...field}
                      onValueChange={field.onChange}
                      placeholder={t('dataFlowPlaceholder')}
                      options={options}
                    />
                  )}
                </>
              </FormControl>
            </div>
          </div>
          <div className="flex pt-1">
            <FormMessage />
          </div>
        </FormItem>
      )}
    />
  );
}
