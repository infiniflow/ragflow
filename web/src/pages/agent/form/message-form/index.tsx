import { FormContainer } from '@/components/form-container';
import { PromptEditor } from '@/components/prompt-editor';
import { BlockButton, Button } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { X } from 'lucide-react';
import { useFieldArray } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { INextOperatorForm } from '../../interface';

const MessageForm = ({ form }: INextOperatorForm) => {
  const { t } = useTranslation();

  const { fields, append, remove } = useFieldArray({
    name: 'content',
    control: form.control,
  });

  return (
    <Form {...form}>
      <form
        className="space-y-5 px-5 "
        autoComplete="off"
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <FormContainer>
          <FormItem>
            <FormLabel tooltip={t('flow.msgTip')}>{t('flow.msg')}</FormLabel>
            <div className="space-y-4">
              {fields.map((field, index) => (
                <div key={field.id} className="flex items-start gap-2">
                  <FormField
                    control={form.control}
                    name={`content.${index}.value`}
                    render={({ field }) => (
                      <FormItem className="flex-1">
                        <FormControl>
                          {/* <Textarea {...field}> </Textarea> */}
                          <PromptEditor
                            {...field}
                            placeholder={t('flow.messagePlaceholder')}
                          ></PromptEditor>
                        </FormControl>
                      </FormItem>
                    )}
                  />
                  {fields.length > 1 && (
                    <Button
                      type="button"
                      variant={'ghost'}
                      onClick={() => remove(index)}
                    >
                      <X />
                    </Button>
                  )}
                </div>
              ))}

              <BlockButton
                type="button"
                onClick={() => append({ value: '' })} // "" will cause the inability to add, refer to: https://github.com/orgs/react-hook-form/discussions/8485#discussioncomment-2961861
              >
                {t('flow.addMessage')}
              </BlockButton>
            </div>
            <FormMessage />
          </FormItem>
        </FormContainer>
      </form>
    </Form>
  );
};

export default MessageForm;
