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
        <FormItem defaultChecked={false}>
          <FormLabel tooltip={t('useGraphRagTip')}>
            {t('useGraphRag')}
          </FormLabel>
          <FormControl>
            <Switch
              checked={field.value}
              onCheckedChange={field.onChange}
            ></Switch>
          </FormControl>
          <FormMessage />
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
              <FormItem>
                <FormLabel
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
                <FormControl>
                  <RAGFlowSelect
                    {...field}
                    options={methodOptions}
                  ></RAGFlowSelect>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="parser_config.graphrag.resolution"
            render={({ field }) => (
              <FormItem>
                <FormLabel tooltip={renderWideTooltip('resolutionTip')}>
                  {t('resolution')}
                </FormLabel>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  ></Switch>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="parser_config.graphrag.community"
            render={({ field }) => (
              <FormItem>
                <FormLabel tooltip={renderWideTooltip('communityTip')}>
                  {t('community')}
                </FormLabel>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  ></Switch>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </>
      )}
    </FormContainer>
  );
};

export default GraphRagItems;
