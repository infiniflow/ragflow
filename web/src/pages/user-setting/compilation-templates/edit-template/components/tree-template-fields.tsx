import { Collapse } from '@/components/collapse';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { SliderInputFormField } from '@/components/slider-input-form-field';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
} from '@/components/ui/form';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { FormLayout } from '@/constants/form';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { FormSchemaType } from '../schema';

type TreeTemplateFieldsProps = {
  index: number;
};

export function TreeTemplateFields({ index }: TreeTemplateFieldsProps) {
  const { t } = useTranslation();

  return (
    <Collapse defaultOpen title={t('setting.raptorTreeSettings')}>
      <div className="space-y-4">
        <RAGFlowFormItem
          name={`templates.${index}.config.raptor.prompt`}
          label={t('setting.summarizationPrompt')}
        >
          <Textarea
            placeholder={t('setting.descriptionPlaceholder')}
            rows={6}
          />
        </RAGFlowFormItem>

        <div className="grid grid-cols-2 gap-4">
          <SliderInputFormField
            name={`templates.${index}.config.raptor.max_token`}
            label={t('setting.maxToken')}
            max={2048}
            min={0}
            step={1}
            layout={FormLayout.Horizontal}
          />
          <SliderInputFormField
            name={`templates.${index}.config.raptor.threshold`}
            label={t('setting.threshold')}
            step={0.01}
            max={1}
            min={0}
            layout={FormLayout.Horizontal}
          />
        </div>

        <RechunkField index={index} />
      </div>
    </Collapse>
  );
}

function RechunkField({ index }: { index: number }) {
  const { t } = useTranslation();
  const form = useFormContext<FormSchemaType>();

  return (
    <FormField
      control={form.control}
      name={`templates.${index}.config.raptor.rechunk`}
      render={({ field }) => (
        <FormItem className="flex items-start justify-between space-y-0 rounded-lg border border-border-button p-4">
          <div className="space-y-1 pr-4">
            <FormLabel className="text-sm text-text-primary">
              {t('setting.rechunkByTreeLeaves')}
            </FormLabel>
            <p className="text-xs text-text-secondary">
              {t('setting.rechunkByTreeLeavesTip')}
            </p>
          </div>
          <FormControl>
            <Switch checked={field.value} onCheckedChange={field.onChange} />
          </FormControl>
        </FormItem>
      )}
    />
  );
}
