import { FormFieldType, RenderField } from '@/components/dynamic-form';
import { SingleFormSlider } from '@/components/ui/dual-range-slider';
import { NumberInput } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { cn } from '@/lib/utils';
import { t } from 'i18next';
import { ListChevronsDownUp, ListChevronsUpDown } from 'lucide-react';
import { useState } from 'react';
import { z } from 'zod';

export const advancedSettingsFormSchema = {
  permission: z.string().optional(),
  storage_type: z.enum(['table', 'graph']).optional(),
  forget_policy: z.enum(['lru', 'fifo']).optional(),
  temperature: z.number().optional(),
  system_prompt: z.string().optional(),
  user_prompt: z.string().optional(),
};
export const defaultAdvancedSettingsForm = {
  permission: 'me',
  storage_type: 'table',
  forget_policy: 'fifo',
  temperature: 0.7,
  system_prompt: '',
  user_prompt: '',
};
export const AdvancedSettingsForm = () => {
  const [showAdvancedSettings, setShowAdvancedSettings] = useState(false);
  return (
    <>
      <div
        className="flex items-center gap-1 w-full cursor-pointer"
        onClick={() => setShowAdvancedSettings(!showAdvancedSettings)}
      >
        {showAdvancedSettings ? (
          <ListChevronsDownUp size={14} />
        ) : (
          <ListChevronsUpDown size={14} />
        )}
        {t('memory.config.advancedSettings')}
      </div>
      {/* {showAdvancedSettings && ( */}
      <>
        <RenderField
          field={{
            name: 'permission',
            label: t('memory.config.permission'),
            required: false,
            horizontal: true,
            // hideLabel: true,
            type: FormFieldType.Custom,
            render: (field) => (
              <RadioGroup
                defaultValue="me"
                className="flex"
                {...field}
                onValueChange={(value) => {
                  console.log(value);
                  field.onChange(value);
                }}
              >
                <div className="flex items-center gap-3">
                  <RadioGroupItem value="me" id="r1" />
                  <Label htmlFor="r1">{t('memory.config.onlyMe')}</Label>
                </div>
                <div className="flex items-center gap-3">
                  <RadioGroupItem value="team" id="r2" />
                  <Label htmlFor="r2">{t('memory.config.team')}</Label>
                </div>
              </RadioGroup>
            ),
          }}
        />
        <RenderField
          field={{
            name: 'storage_type',
            label: t('memory.config.storageType'),
            type: FormFieldType.Select,
            horizontal: true,
            placeholder: t('memory.config.storageTypePlaceholder'),
            options: [
              { label: 'table', value: 'table' },
              // { label: 'graph', value: 'graph' },
            ],
            required: false,
          }}
        />
        <RenderField
          field={{
            name: 'forget_policy',
            label: t('memory.config.forgetPolicy'),
            type: FormFieldType.Select,
            horizontal: true,
            // placeholder: t('memory.config.storageTypePlaceholder'),
            options: [
              { label: 'lru', value: 'lru' },
              { label: 'fifo', value: 'fifo' },
            ],
            required: false,
          }}
        />
        <RenderField
          field={{
            name: 'temperature',
            label: t('memory.config.temperature'),
            type: FormFieldType.Custom,
            horizontal: true,
            required: false,
            render: (field) => (
              <div className="flex gap-2 items-center">
                <SingleFormSlider
                  {...field}
                  onChange={(value: number) => {
                    field.onChange(value);
                  }}
                  max={1}
                  step={0.01}
                  min={0}
                  disabled={false}
                ></SingleFormSlider>
                <NumberInput
                  className={cn(
                    'h-6 w-10 p-1 border border-border-button rounded-sm',
                  )}
                  max={1}
                  step={0.01}
                  min={0}
                  {...field}
                ></NumberInput>
              </div>
            ),
          }}
        />
        <RenderField
          field={{
            name: 'system_prompt',
            label: t('memory.config.systemPrompt'),
            type: FormFieldType.Textarea,
            horizontal: true,
            placeholder: t('memory.config.systemPromptPlaceholder'),
            required: false,
          }}
        />
        <RenderField
          field={{
            name: 'user_prompt',
            label: t('memory.config.userPrompt'),
            type: FormFieldType.Text,
            horizontal: true,
            placeholder: t('memory.config.userPromptPlaceholder'),
            required: false,
          }}
        />
      </>
      {/* )} */}
    </>
  );
};
