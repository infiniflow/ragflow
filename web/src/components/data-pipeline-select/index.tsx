import { useTranslate } from '@/hooks/common-hooks';
import { buildSelectOptions } from '@/utils/component-util';
import { ArrowUpRight } from 'lucide-react';
import { useFormContext } from 'react-hook-form';
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
}

const data = [
  { id: '1', name: 'data-pipeline-1' },
  { id: '2', name: 'data-pipeline-2' },
  { id: '3', name: 'data-pipeline-3' },
  { id: '4', name: 'data-pipeline-4' },
];
export function DataFlowSelect(props: IProps) {
  const { toDataPipeline, formFieldName } = props;
  const { t } = useTranslate('knowledgeConfiguration');
  const form = useFormContext();
  console.log('data-pipline form', form);
  const toDataPipLine = () => {
    toDataPipeline?.();
  };
  const options = buildSelectOptions(data, 'id', 'name');
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
              <div
                className="text-sm flex text-text-primary cursor-pointer"
                onClick={toDataPipLine}
              >
                {t('buildItFromScratch')}
                <ArrowUpRight size={14} />
              </div>
            </div>

            <div className="text-muted-foreground">
              <FormControl>
                <MultiSelect
                  {...field}
                  onValueChange={field.onChange}
                  placeholder={t('dataFlowPlaceholder')}
                  options={options}
                />
              </FormControl>
            </div>
          </div>
          <div className="flex pt-1">
            <div className="w-full"></div>
            <FormMessage />
          </div>
        </FormItem>
      )}
    />
  );
}
