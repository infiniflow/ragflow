import { DocumentParserType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import { cn } from '@/lib/utils';
import { Switch as AntSwitch, Form, Select } from 'antd';
import { upperFirst } from 'lodash';
import { useCallback, useMemo } from 'react';
import { useFormContext } from 'react-hook-form';
import { DatasetConfigurationContainer } from '../dataset-configuration-container';
import EntityTypesItem from '../entity-types-item';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '../ui/form';
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
};

export function UseGraphRagItem() {
  const { t } = useTranslate('knowledgeConfiguration');

  return (
    <Form.Item
      name={['parser_config', 'graphrag', 'use_graphrag']}
      label={t('useGraphRag')}
      initialValue={false}
      valuePropName="checked"
      tooltip={t('useGraphRagTip')}
    >
      <AntSwitch />
    </Form.Item>
  );
}

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
const GraphRagItems = ({ marginBottom = false }: GraphRagItemsProps) => {
  const { t } = useTranslate('knowledgeConfiguration');

  const methodOptions = useMemo(() => {
    return [MethodValue.Light, MethodValue.General].map((x) => ({
      value: x,
      label: upperFirst(x),
    }));
  }, []);

  const renderWideTooltip = useCallback(
    (title: React.ReactNode | string) => {
      return {
        title: typeof title === 'string' ? t(title) : title,
        overlayInnerStyle: { width: '32vw' },
      };
    },
    [t],
  );

  return (
    <DatasetConfigurationContainer className={cn({ 'mb-4': marginBottom })}>
      <UseGraphRagItem></UseGraphRagItem>
      <Form.Item
        shouldUpdate={(prevValues, curValues) =>
          prevValues.parser_config.graphrag.use_graphrag !==
          curValues.parser_config.graphrag.use_graphrag
        }
      >
        {({ getFieldValue }) => {
          const useRaptor = getFieldValue([
            'parser_config',
            'graphrag',
            'use_graphrag',
          ]);

          return (
            useRaptor && (
              <>
                <EntityTypesItem
                  field={['parser_config', 'graphrag', 'entity_types']}
                ></EntityTypesItem>
                <Form.Item
                  name={['parser_config', 'graphrag', 'method']}
                  label={t('graphRagMethod')}
                  tooltip={renderWideTooltip(
                    <div
                      dangerouslySetInnerHTML={{
                        __html: t('graphRagMethodTip'),
                      }}
                    ></div>,
                  )}
                  initialValue={MethodValue.Light}
                >
                  <Select options={methodOptions} />
                </Form.Item>
                <Form.Item
                  name={['parser_config', 'graphrag', 'resolution']}
                  label={t('resolution')}
                  tooltip={renderWideTooltip('resolutionTip')}
                >
                  <AntSwitch />
                </Form.Item>
                <Form.Item
                  name={['parser_config', 'graphrag', 'community']}
                  label={t('community')}
                  tooltip={renderWideTooltip('communityTip')}
                >
                  <AntSwitch />
                </Form.Item>
              </>
            )
          );
        }}
      </Form.Item>
    </DatasetConfigurationContainer>
  );
};

export default GraphRagItems;
