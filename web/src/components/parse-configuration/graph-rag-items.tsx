import { useTranslate } from '@/hooks/common-hooks';
import { Divider, Form, Select, Switch } from 'antd';
import { upperFirst } from 'lodash';
import { useCallback, useMemo } from 'react';
import EntityTypesItem from '../entity-types-item';

const excludedTagParseMethods = ['table', 'knowledge_graph', 'tag'];

export const showTagItems = (parserId: string) => {
  return !excludedTagParseMethods.includes(parserId);
};

const enum MethodValue {
  General = 'general',
  Light = 'light',
}

export const excludedParseMethods = [
  'table',
  'resume',
  'picture',
  'knowledge_graph',
  'qa',
  'tag',
];

export const showGraphRagItems = (parserId: string) => {
  return !excludedParseMethods.includes(parserId);
};

// The three types "table", "resume" and "one" do not display this configuration.
const GraphRagItems = () => {
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
        overlayInnerStyle: { width: '50vw' },
      };
    },
    [t],
  );

  return (
    <>
      <Divider></Divider>
      <Form.Item
        name={['parser_config', 'graphrag', 'use_graphrag']}
        label={t('useGraphRag')}
        initialValue={false}
        valuePropName="checked"
        tooltip={renderWideTooltip('useGraphRagTip')}
      >
        <Switch />
      </Form.Item>
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
                  <Switch />
                </Form.Item>
                <Form.Item
                  name={['parser_config', 'graphrag', 'community']}
                  label={t('community')}
                  tooltip={renderWideTooltip('communityTip')}
                >
                  <Switch />
                </Form.Item>
              </>
            )
          );
        }}
      </Form.Item>
    </>
  );
};

export default GraphRagItems;
