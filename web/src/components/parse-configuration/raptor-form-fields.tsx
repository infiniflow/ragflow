import { FormLayout } from '@/constants/form';
import { DocumentParserType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import random from 'lodash/random';
import { Plus } from 'lucide-react';
import { useCallback } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { SliderInputFormField } from '../slider-input-form-field';
import { Button } from '../ui/button';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '../ui/form';
import { Input } from '../ui/input';
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
const MaxTokenField = 'parser_config.raptor.max_token';
const ThresholdField = 'parser_config.raptor.threshold';
const MaxCluster = 'parser_config.raptor.max_cluster';
const Prompt = 'parser_config.raptor.prompt';

// The three types "table", "resume" and "one" do not display this configuration.

const RaptorFormFields = () => {
  const form = useFormContext();
  const { t } = useTranslate('knowledgeConfiguration');
  const useRaptor = useWatch({ name: UseRaptorField });

  const changeRaptor = useCallback(
    (isUseRaptor: boolean) => {
      if (isUseRaptor) {
        form.setValue(MaxTokenField, 256);
        form.setValue(ThresholdField, 0.1);
        form.setValue(MaxCluster, 64);
        form.setValue(RandomSeedField, 0);
        form.setValue(Prompt, t('promptText'));
      }
    },
    [form],
  );

  const handleGenerate = useCallback(() => {
    form.setValue(RandomSeedField, random(10000));
  }, [form]);

  return (
    <>
      <FormField
        control={form.control}
        name={UseRaptorField}
        render={({ field }) => {
          // if (typeof field.value === 'undefined') {
          //   // default value set
          //   form.setValue('parser_config.raptor.use_raptor', false);
          // }
          return (
            <FormItem
              defaultChecked={false}
              className="items-center space-y-0 "
            >
              <div className="flex items-center gap-1">
                <FormLabel
                  tooltip={t('useRaptorTip')}
                  className="text-sm text-muted-foreground w-1/4 whitespace-break-spaces"
                >
                  {t('useRaptor')}
                </FormLabel>
                <div className="w-3/4">
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={(e) => {
                        changeRaptor(e);
                        field.onChange(e);
                      }}
                    ></Switch>
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
                      className="text-sm text-muted-foreground whitespace-nowrap w-1/4"
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
          ></SliderInputFormField>
          <SliderInputFormField
            name={'parser_config.raptor.threshold'}
            label={t('threshold')}
            tooltip={t('thresholdTip')}
            step={0.01}
            max={1}
            min={0}
            layout={FormLayout.Horizontal}
          ></SliderInputFormField>
          <SliderInputFormField
            name={'parser_config.raptor.max_cluster'}
            label={t('maxCluster')}
            tooltip={t('maxClusterTip')}
            max={1024}
            min={1}
            layout={FormLayout.Horizontal}
          ></SliderInputFormField>
          <FormField
            control={form.control}
            name={'parser_config.raptor.random_seed'}
            render={({ field }) => (
              <FormItem className=" items-center space-y-0 ">
                <div className="flex items-center">
                  <FormLabel className="text-sm text-muted-foreground whitespace-nowrap w-1/4">
                    {t('randomSeed')}
                  </FormLabel>
                  <div className="w-3/4">
                    <FormControl defaultValue={0}>
                      <div className="flex gap-4">
                        <Input {...field} defaultValue={0} />
                        <Button
                          size={'sm'}
                          onClick={handleGenerate}
                          type={'button'}
                        >
                          <Plus />
                        </Button>
                      </div>
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
