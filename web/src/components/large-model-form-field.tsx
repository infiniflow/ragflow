import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { LlmModelType } from '@/constants/knowledge';
import { t } from 'i18next';
import { Funnel } from 'lucide-react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { NextInnerLLMSelectProps, NextLLMSelect } from './llm-select/next';
import { Button } from './ui/button';

const ModelTypes = [
  {
    title: t('flow.allModels'),
    value: 'all',
  },
  {
    title: t('flow.textOnlyModels'),
    value: LlmModelType.Chat,
  },
  {
    title: t('flow.multimodalModels'),
    value: LlmModelType.Image2text,
  },
];

export const LargeModelFilterFormSchema = {
  llm_filter: z.string().optional(),
};

type LargeModelFormFieldProps = Pick<
  NextInnerLLMSelectProps,
  'showSpeech2TextModel'
>;
export function LargeModelFormField({
  showSpeech2TextModel: showTTSModel,
}: LargeModelFormFieldProps) {
  const form = useFormContext();
  const { t } = useTranslation();
  const filter = useWatch({ control: form.control, name: 'llm_filter' });

  return (
    <>
      <FormField
        control={form.control}
        name="llm_id"
        render={({ field }) => (
          <FormItem>
            <FormLabel tooltip={t('chat.modelTip')}>
              {t('chat.model')}
            </FormLabel>
            <section className="flex gap-2.5">
              <FormField
                control={form.control}
                name="llm_filter"
                render={({ field }) => (
                  <FormItem>
                    <FormControl>
                      <DropdownMenu>
                        <DropdownMenuTrigger>
                          <Button variant={'ghost'}>
                            <Funnel className="text-text-disabled" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent>
                          {ModelTypes.map((x) => (
                            <DropdownMenuItem
                              key={x.value}
                              onClick={() => {
                                field.onChange(x.value);
                              }}
                            >
                              {x.title}
                            </DropdownMenuItem>
                          ))}
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </FormControl>
                  </FormItem>
                )}
              />

              <FormControl>
                <NextLLMSelect
                  {...field}
                  filter={filter}
                  showSpeech2TextModel={showTTSModel}
                />
              </FormControl>
            </section>

            <FormMessage />
          </FormItem>
        )}
      />
    </>
  );
}

export function LargeModelFormFieldWithoutFilter() {
  const form = useFormContext();

  return (
    <FormField
      control={form.control}
      name="llm_id"
      render={({ field }) => (
        <FormItem>
          <FormControl>
            <NextLLMSelect {...field} />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
