import { DocumentParserType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import { cn } from '@/lib/utils';
import { upperFirst } from 'lodash';
import { useCallback, useMemo } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { EntityTypesFormField } from '../entity-types-form-field';
import { FormContainer } from '../form-container';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '../ui/form';
import { RAGFlowSelect } from '../ui/select';
import { Switch } from '../ui/switch';

const excludedTagParseMethods = [
  DocumentParserType.Table,
  DocumentParserType.KnowledgeGraph,
  DocumentParserType.Tag,
];

export const showTagItems = (parserId: DocumentParserType) => {
  return !excludedTagParseMethods.includes(parserId);
};

const enum MethodValue {
  General = 'general',
  Light = 'light',
}

export const excludedParseMethods = [
  DocumentParserType.Table,
  DocumentParserType.Resume,
  DocumentParserType.Picture,
  DocumentParserType.KnowledgeGraph,
  DocumentParserType.Qa,
  DocumentParserType.Tag,
];

export const showGraphRagItems = (parserId: DocumentParserType | undefined) => {
  return !excludedParseMethods.some((x) => x === parserId);
};

type GraphRagItemsProps = {
  marginBottom?: boolean;
  className?: string;
};

export function UseGraphRagFormField() {
  const form = useFormContext();
  const { t } = useTranslate('knowledgeConfiguration');

  return (
    <FormField
      control={form.control}
      name="parser_config.graphrag.use_graphrag"
      render={({ field }) => (
        <FormItem defaultChecked={false} className=" items-center space-y-0 ">
          <div className="flex items-center gap-1">
            <FormLabel
              tooltip={t('useGraphRagTip')}
              className="text-sm text-muted-foreground whitespace-break-spaces w-1/4"
            >
              {t('useGraphRag')}
            </FormLabel>
            <div className="w-3/4">
              <FormControl>
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                ></Switch>
              </FormControl>
            </div>
          </div>
          <div className="flex pt-1">
            <div className="w-1/4"></div>
            <FormMessage />
          </div>
        </FormItem>
      )}
    />
  );
}

// The three types "table", "resume" and "one" do not display this configuration.
const GraphRagItems = ({
  marginBottom = false,
  className = 'p-10',
}: GraphRagItemsProps) => {
  const { t } = useTranslate('knowledgeConfiguration');
  const form = useFormContext();

  const useRaptor = useWatch({
    control: form.control,
    name: 'parser_config.graphrag.use_graphrag',
  });

  const methodOptions = useMemo(() => {
    return [MethodValue.Light, MethodValue.General].map((x) => ({
      value: x,
      label: upperFirst(x),
    }));
  }, []);

  const renderWideTooltip = useCallback(
    (title: React.ReactNode | string) => {
      return typeof title === 'string' ? t(title) : title;
    },
    [t],
  );

  return (
    <FormContainer className={cn({ 'mb-4': marginBottom }, className)}>
      <UseGraphRagFormField></UseGraphRagFormField>
      {useRaptor && (
        <>
          <EntityTypesFormField name="parser_config.graphrag.entity_types"></EntityTypesFormField>
          <FormField
            control={form.control}
            name="parser_config.graphrag.method"
            render={({ field }) => (
              <FormItem className=" items-center space-y-0 ">
                <div className="flex items-center">
                  <FormLabel
                    className="text-sm text-muted-foreground whitespace-nowrap w-1/4"
                    tooltip={renderWideTooltip(
                      <div
                        dangerouslySetInnerHTML={{
                          __html: t('graphRagMethodTip'),
                        }}
                      ></div>,
                    )}
                  >
                    {t('graphRagMethod')}
                  </FormLabel>
                  <div className="w-3/4">
                    <FormControl>
                      <RAGFlowSelect
                        {...field}
                        options={methodOptions}
                      ></RAGFlowSelect>
                    </FormControl>
                  </div>
                </div>
                <div className="flex pt-1">
                  <div className="w-1/4"></div>
                  <FormMessage />
                </div>
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="parser_config.graphrag.resolution"
            render={({ field }) => (
              <FormItem className=" items-center space-y-0 ">
                <div className="flex items-center">
                  <FormLabel
                    tooltip={renderWideTooltip('resolutionTip')}
                    className="text-sm text-muted-foreground whitespace-nowrap w-1/4"
                  >
                    {t('resolution')}
                  </FormLabel>
                  <div className="w-3/4">
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      ></Switch>
                    </FormControl>
                  </div>
                </div>
                <div className="flex pt-1">
                  <div className="w-1/4"></div>
                  <FormMessage />
                </div>
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="parser_config.graphrag.community"
            render={({ field }) => (
              <FormItem className=" items-center space-y-0 ">
                <div className="flex items-center">
                  <FormLabel
                    tooltip={renderWideTooltip('communityTip')}
                    className="text-sm text-muted-foreground whitespace-nowrap w-1/4"
                  >
                    {t('community')}
                  </FormLabel>
                  <div className="w-3/4">
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      ></Switch>
                    </FormControl>
                  </div>
                </div>
                <div className="flex pt-1">
                  <div className="w-1/4"></div>
                  <FormMessage />
                </div>
              </FormItem>
            )}
          />
        </>
      )}
    </FormContainer>
  );
};

export default GraphRagItems;
