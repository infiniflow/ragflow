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
import { Funnel } from 'lucide-react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { NextLLMSelect } from './llm-select/next';
import { Button } from './ui/button';

const ModelTypes = [
  {
    title: 'All Models',
    value: 'all',
  },
  {
    title: 'Text-only Models',
    value: LlmModelType.Chat,
  },
  {
    title: 'Multimodal Models',
    value: LlmModelType.Image2text,
  },
];

export const LargeModelFilterFormSchema = {
  llm_filter: z.string().optional(),
};

export function LargeModelFormField() {
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
                            <Funnel />
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
                <NextLLMSelect {...field} filter={filter} />
              </FormControl>
            </section>

            <FormMessage />
          </FormItem>
        )}
      />
    </>
  );
}
