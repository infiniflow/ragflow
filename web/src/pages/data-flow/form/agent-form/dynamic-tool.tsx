import { BlockButton, Button } from '@/components/ui/button';
import {
  FormControl,
  FormField,
  FormItem,
  FormMessage,
} from '@/components/ui/form';
import { X } from 'lucide-react';
import { memo } from 'react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { PromptEditor } from '../components/prompt-editor';

const DynamicTool = () => {
  const form = useFormContext();
  const name = 'tools';

  const { fields, append, remove } = useFieldArray({
    name: name,
    control: form.control,
  });

  return (
    <FormItem>
      <div className="space-y-4">
        {fields.map((field, index) => (
          <div key={field.id} className="flex">
            <div className="space-y-2 flex-1">
              <FormField
                control={form.control}
                name={`${name}.${index}.component_name`}
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
      <BlockButton onClick={() => append({ component_name: '' })}>
        Add
      </BlockButton>
    </FormItem>
  );
};

export default memo(DynamicTool);
