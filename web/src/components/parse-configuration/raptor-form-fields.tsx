import { FormLayout } from '@/constants/form';
import { DocumentParserType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import { IGenerateLogButtonProps } from '@/pages/dataset/dataset/generate-button/generate';
import random from 'lodash/random';
import { Shuffle } from 'lucide-react';
import { useCallback, useEffect, useMemo } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { SliderInputFormField } from '../slider-input-form-field';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '../ui/form';
import { ExpandedInput } from '../ui/input';
import { Radio } from '../ui/radio';
import { Switch } from '../ui/switch';
import { Textarea } from '../ui/textarea';

export const excludedParseMethods = [
  DocumentParserType.Table,
  DocumentParserType.Resume,
  DocumentParserType.One,
  DocumentParserType.Picture,
  DocumentParserType.KnowledgeGraph,
  DocumentParserType.Qa,
  DocumentParserType.Tag,
];

export const showRaptorParseConfiguration = (
  parserId: DocumentParserType | undefined,
) => {
  return !excludedParseMethods.some((x) => x === parserId);
};

export const excludedTagParseMethods = [
  DocumentParserType.Table,
  DocumentParserType.KnowledgeGraph,
  DocumentParserType.Tag,
];

export const showTagItems = (parserId: DocumentParserType) => {
  return !excludedTagParseMethods.includes(parserId);
};

const UseRaptorField = 'parser_config.raptor.use_raptor';
const RandomSeedField = 'parser_config.raptor.random_seed';
const ClusteringMethodField = 'parser_config.raptor.clustering_method';
const ClusteringMethodExtField = 'parser_config.raptor.ext.clustering_method';
const TreeBuilderField = 'parser_config.raptor.tree_builder';
const ScopeField = 'parser_config.raptor.scope';
const MaxClusterMax = 1024;
/**
 * Generation scope is hard-coded to "file": RAPTOR now always runs per
 * document. The Radio that used to expose dataset / file is gone, but
 * the field is still kept on the form so the value flows through to
 * the backend exactly like before.
 */
const FIXED_SCOPE = 'file';

// The three types "table", "resume" and "one" do not display this configuration.

const RaptorFormFields = ({
  data: _data,
  onDelete: _onDelete,
}: {
  data: IGenerateLogButtonProps;
  onDelete: () => void;
}) => {
  // ``data`` / ``onDelete`` are kept on the prop signature for
  // call-site compatibility but no longer used — the "RAPTOR: Not
  // generated" log stub they fed has been dropped. The "Use RAPTOR"
  // control is now a real Switch (defaults to off) and the rest of
  // the configuration only renders when it's toggled on.
  void _data;
  void _onDelete;
  const form = useFormContext();
  const { t } = useTranslate('knowledgeConfiguration');
  const useRaptor = useWatch({ name: UseRaptorField });
  const clusteringMethod = useWatch({ name: ClusteringMethodField });
  const extClusteringMethod = useWatch({ name: ClusteringMethodExtField });
  const selectedClusteringMethod = useMemo(
    () =>
      (clusteringMethod ??
        extClusteringMethod ??
        form.getValues(ClusteringMethodField) ??
        form.getValues(ClusteringMethodExtField) ??
        'gmm') as 'gmm' | 'ahc',
    [clusteringMethod, extClusteringMethod, form],
  );

  const handleGenerate = useCallback(() => {
    form.setValue(RandomSeedField, random(10000));
  }, [form]);

  const handleClusteringMethodChange = useCallback(
    (method: 'gmm' | 'ahc') => {
      form.setValue(ClusteringMethodField, method, {
        shouldDirty: true,
        shouldValidate: true,
      });
      form.setValue(TreeBuilderField, 'raptor', {
        shouldDirty: true,
        shouldValidate: true,
      });
    },
    [form],
  );

  useEffect(() => {
    if (!clusteringMethod && !extClusteringMethod) {
      handleClusteringMethodChange('gmm');
    }
  }, [clusteringMethod, extClusteringMethod, handleClusteringMethodChange]);

  // Force-pin the (now invisible) generation scope to "file" the first
  // time this form mounts. Without this the field stays undefined and
  // the backend's default branch kicks in instead.
  useEffect(() => {
    if (form.getValues(ScopeField) !== FIXED_SCOPE) {
      form.setValue(ScopeField, FIXED_SCOPE, {
        shouldDirty: false,
        shouldValidate: false,
      });
    }
    // Seed ``use_raptor`` to an explicit ``false`` when it's missing
    // entirely — i.e. only on first mount for docs that have no prior
    // raptor block. The ``=== undefined`` guard means we leave a
    // stored ``true`` (or an already-set ``false``) untouched, so
    // toggling later doesn't snap back to the default.
    if (form.getValues(UseRaptorField) === undefined) {
      form.setValue(UseRaptorField, false, {
        shouldDirty: false,
        shouldValidate: false,
      });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <>
      <FormField
        control={form.control}
        name={UseRaptorField}
        render={({ field }) => {
          return (
            <FormItem className="items-center space-y-0 ">
              <div className="flex items-center gap-1">
                <FormLabel
                  tooltip={t('useRaptorTip')}
                  className="text-sm  w-1/4 whitespace-break-spaces"
                >
                  {t('useRaptor')}
                </FormLabel>
                <div className="w-3/4">
                  <FormControl>
                    {/* Default-off Switch. The detailed RAPTOR config
                        (prompt, max token, …) renders only when this
                        is toggled on — see the gated block below. */}
                    <Switch
                      checked={!!field.value}
                      onCheckedChange={field.onChange}
                      data-testid="ds-settings-raptor-use-raptor-switch"
                    />
                  </FormControl>
                </div>
              </div>
              <div className="flex pt-1">
                <div className="w-1/4"></div>
                <FormMessage />
              </div>
            </FormItem>
          );
        }}
      />
      {/* Generation scope is hard-coded to "file" — no Radio surface.
          The field is still on the form via useEffect above so the
          value flows to the backend on submit. */}
      {useRaptor && (
        <div className="space-y-3">
          <FormField
            control={form.control}
            name={'parser_config.raptor.prompt'}
            render={({ field }) => {
              return (
                <FormItem className=" items-center space-y-0 ">
                  <div className="flex items-start">
                    <FormLabel
                      tooltip={t('promptTip')}
                      className="text-sm  whitespace-nowrap w-1/4"
                    >
                      {t('prompt')}
                    </FormLabel>
                    <div className="w-3/4">
                      <FormControl>
                        <Textarea
                          {...field}
                          rows={8}
                          onChange={(e) => {
                            field.onChange(e?.target?.value);
                          }}
                          data-testid="ds-settings-raptor-prompt-textarea"
                        />
                      </FormControl>
                    </div>
                  </div>
                  <div className="flex pt-1">
                    <div className="w-1/4"></div>
                    <FormMessage />
                  </div>
                </FormItem>
              );
            }}
          />
          <SliderInputFormField
            name={'parser_config.raptor.max_token'}
            label={t('maxToken')}
            tooltip={t('maxTokenTip')}
            max={2048}
            min={0}
            layout={FormLayout.Horizontal}
            sliderTestId="ds-settings-raptor-max-token-slider"
            numberInputTestId="ds-settings-raptor-max-token-input"
          ></SliderInputFormField>
          <SliderInputFormField
            name={'parser_config.raptor.threshold'}
            label={t('threshold')}
            tooltip={t('thresholdTip')}
            step={0.01}
            max={1}
            min={0}
            layout={FormLayout.Horizontal}
            sliderTestId="ds-settings-raptor-threshold-slider"
            numberInputTestId="ds-settings-raptor-threshold-input"
          ></SliderInputFormField>
          <FormField
            control={form.control}
            name={ClusteringMethodField}
            render={({ field }) => {
              return (
                <FormItem className=" items-center space-y-0 ">
                  <div className="flex items-start">
                    <FormLabel
                      tooltip={t('clusteringMethodTip')}
                      className="text-sm  whitespace-nowrap w-1/4"
                    >
                      {t('clusteringMethod')}
                    </FormLabel>
                    <div className="w-3/4">
                      <FormControl>
                        <Radio.Group
                          {...field}
                          value={selectedClusteringMethod}
                          onChange={(value) =>
                            handleClusteringMethodChange(value as 'gmm' | 'ahc')
                          }
                        >
                          <div
                            className={'flex gap-4 w-full text-text-secondary '}
                          >
                            <Radio
                              value="gmm"
                              testId="ds-settings-raptor-clustering-method-option-gmm"
                            >
                              {t('clusteringMethodGmm')}
                            </Radio>
                            <Radio
                              value="ahc"
                              testId="ds-settings-raptor-clustering-method-option-ahc"
                            >
                              {t('clusteringMethodAhc')}
                            </Radio>
                          </div>
                        </Radio.Group>
                      </FormControl>
                    </div>
                  </div>
                  <div className="flex pt-1">
                    <div className="w-1/4"></div>
                    <FormMessage />
                  </div>
                </FormItem>
              );
            }}
          />
          <SliderInputFormField
            name={'parser_config.raptor.max_cluster'}
            label={t('maxCluster')}
            tooltip={t('maxClusterTip')}
            max={MaxClusterMax}
            min={1}
            layout={FormLayout.Horizontal}
            sliderTestId="ds-settings-raptor-max-cluster-slider"
            numberInputTestId="ds-settings-raptor-max-cluster-input"
          ></SliderInputFormField>
          <FormField
            control={form.control}
            name={'parser_config.raptor.random_seed'}
            render={({ field }) => (
              <FormItem className=" items-center space-y-0 ">
                <div className="flex items-center">
                  <FormLabel
                    className="text-sm  whitespace-wrap w-1/4"
                    tooltip={t('randomSeedTip')}
                  >
                    {t('randomSeed')}
                  </FormLabel>
                  <div className="w-3/4">
                    <FormControl defaultValue={0}>
                      <ExpandedInput
                        {...field}
                        className="w-full"
                        defaultValue={0}
                        type="number"
                        data-testid="ds-settings-raptor-seed-input"
                        suffix={
                          <div className="w-7 flex justify-center items-center">
                            <Shuffle
                              className="size-3.5 cursor-pointer"
                              onClick={handleGenerate}
                              data-testid="ds-settings-raptor-seed-randomize-btn"
                            />
                          </div>
                        }
                      />
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
        </div>
      )}
    </>
  );
};

export default RaptorFormFields;
