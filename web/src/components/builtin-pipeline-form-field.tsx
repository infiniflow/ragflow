import { SelectWithSearch } from '@/components/originui/select-with-search';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { useFetchBuiltinPipelines } from '@/hooks/use-agent-request';
import { cn } from '@/lib/utils';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

export function BuiltinPipelineItem({
  line = 2,
  name = 'parser_id',
}: {
  line?: 1 | 2;
  name?: string;
}) {
  const { t } = useTranslation();
  const form = useFormContext();
  const { options: builtinPipelineOptions } = useFetchBuiltinPipelines();

  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem className="items-center space-y-0">
          <div
            className={cn('flex', {
              'items-center': line === 1,
              'flex-col gap-1': line === 2,
            })}
          >
            <FormLabel
              required
              className={cn('text-sm whitespace-wrap', {
                'w-1/4': line === 1,
              })}
            >
              {t('knowledgeConfiguration.builtIn')}
            </FormLabel>
            <div
              className={cn('text-muted-foreground', { 'w-3/4': line === 1 })}
            >
              <FormControl>
                <SelectWithSearch
                  {...field}
                  placeholder={t(
                    'knowledgeConfiguration.chunkMethodPlaceholder',
                  )}
                  options={builtinPipelineOptions}
                />
              </FormControl>
            </div>
          </div>
          <div className="flex pt-1">
            <div className={line === 1 ? 'w-1/4' : ''}></div>
            <FormMessage />
          </div>
        </FormItem>
      )}
    />
  );
}
