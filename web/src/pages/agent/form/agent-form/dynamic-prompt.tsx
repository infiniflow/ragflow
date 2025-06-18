import { BlockButton, Button } from '@/components/ui/button';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { RAGFlowSelect } from '@/components/ui/select';
import { X } from 'lucide-react';
import { memo } from 'react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { PromptRole } from '../../constant';
import { PromptEditor } from '../components/prompt-editor';

const options = [
  { label: 'User', value: PromptRole.User },
  { label: 'Assistant', value: PromptRole.Assistant },
];

const DynamicPrompt = () => {
  const { t } = useTranslation();
  const form = useFormContext();
  const name = 'prompts';

  const { fields, append, remove } = useFieldArray({
    name: name,
    control: form.control,
  });

  return (
    <FormItem>
      <FormLabel tooltip={t('flow.msgTip')}>{t('flow.msg')}</FormLabel>
      <div className="space-y-4">
        {fields.map((field, index) => (
          <div key={field.id} className="flex">
            <div className="space-y-2 flex-1">
              <FormField
                control={form.control}
                name={`${name}.${index}.role`}
                render={({ field }) => (
                  <FormItem className="w-1/3">
                    <FormLabel />
                    <FormControl>
                      <RAGFlowSelect
                        {...field}
                        options={options}
                      ></RAGFlowSelect>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name={`${name}.${index}.content`}
                render={({ field }) => (
                  <FormItem className="flex-1">
                    <FormControl>
                      <section>
                        <PromptEditor
                          {...field}
                          showToolbar={false}
                        ></PromptEditor>
                      </section>
                    </FormControl>
                  </FormItem>
                )}
              />
            </div>
            <Button
              type="button"
              variant={'ghost'}
              onClick={() => remove(index)}
            >
              <X />
            </Button>
          </div>
        ))}
      </div>
      <FormMessage />
      <BlockButton
        onClick={() => append({ content: '', role: PromptRole.User })}
      >
        Add
      </BlockButton>
    </FormItem>
  );
};

export default memo(DynamicPrompt);
