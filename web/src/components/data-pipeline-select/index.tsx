import { AgentCategory } from '@/constants/agent';
import { FormLayout } from '@/constants/form';
import { useTranslate } from '@/hooks/common-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchAgentList } from '@/hooks/use-agent-request';
import { buildSelectOptions } from '@/utils/component-util';
import { ArrowUpRight } from 'lucide-react';
import { useEffect, useMemo } from 'react';
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
export interface IDataPipelineSelectNode {
  id?: string;
  name?: string;
  avatar?: string;
}

interface IProps {
  showToDataPipeline?: boolean;
  formFieldName: string;
  isMult?: boolean;
  setDataList?: (data: IDataPipelineSelectNode[]) => void;
  layout?: FormLayout;
}

export function DataFlowSelect(props: IProps) {
  const {
    showToDataPipeline,
    formFieldName,
    isMult = false,
    setDataList,
    layout = FormLayout.Vertical,
  } = props;

  const { t } = useTranslate('knowledgeConfiguration');
  const form = useFormContext();
  const { navigateToAgents } = useNavigatePage();
  const toDataPipLine = () => {
    navigateToAgents();
  };
  const { data: dataPipelineOptions } = useFetchAgentList({
    canvas_category: AgentCategory.DataflowCanvas,
  });
  const options = useMemo(() => {
    const option = buildSelectOptions(
      dataPipelineOptions?.canvas,
      'id',
      'title',
    );

    return option || [];
  }, [dataPipelineOptions]);

  const nodes = useMemo(() => {
    return (
      dataPipelineOptions?.canvas?.map((item) => {
        return {
          id: item?.id,
          name: item?.title,
          avatar: item?.avatar,
        };
      }) || []
    );
  }, [dataPipelineOptions]);

  useEffect(() => {
    setDataList?.(nodes);
  }, [nodes, setDataList]);

  return (
    <FormField
      control={form.control}
      name={formFieldName}
      render={({ field }) => (
        <FormItem className=" items-center space-y-0 ">
          {layout === FormLayout.Vertical && (
            <div className="flex flex-col gap-1">
              <div className="flex gap-2 justify-between ">
                <FormLabel
                  // tooltip={t('dataFlowTip')}
                  className="text-sm text-text-primary whitespace-wrap "
                >
                  {t('manualSetup')}
                </FormLabel>
                {showToDataPipeline && (
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
                        triggerClassName="!bg-bg-base"
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
          )}
          {layout === FormLayout.Horizontal && (
            <div className="flex gap-1 items-center">
              <div className="flex gap-2 justify-between w-1/4">
                <FormLabel
                  // tooltip={t('dataFlowTip')}
                  className="text-sm text-text-secondary whitespace-wrap "
                >
                  {t('manualSetup')}
                </FormLabel>
              </div>

              <div className="text-muted-foreground w-3/4 flex flex-col items-end">
                {showToDataPipeline && (
                  <div
                    className="text-sm flex text-text-primary cursor-pointer"
                    onClick={toDataPipLine}
                  >
                    {t('buildItFromScratch')}
                    <ArrowUpRight size={14} />
                  </div>
                )}
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
          )}
          <div className="flex pt-1">
            <FormMessage />
          </div>
        </FormItem>
      )}
    />
  );
}
