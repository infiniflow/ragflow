import { Button } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Textarea } from '@/components/ui/textarea';
import { PlusCircle, Trash2 } from 'lucide-react';
import { useFieldArray } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { INextOperatorForm } from '../../interface';

const MessageForm = ({ form }: INextOperatorForm) => {
  const { t } = useTranslation();
  const { fields, append, remove } = useFieldArray({
    name: 'messages',
    control: form.control,
  });

  return (
    <Form {...form}>
      <form
        className="space-y-6"
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <FormItem>
          <FormLabel>{t('flow.msg')}</FormLabel>
          <div className="space-y-4">
            {fields.map((field, index) => (
              <div key={field.id} className="flex items-start gap-2">
                <FormField
                  control={form.control}
                  name={`messages.${index}`}
                  render={({ field }) => (
                    <FormItem className="flex-1">
                      <FormControl>
                        <Textarea
                          {...field}
                          placeholder={t('flow.messagePlaceholder')}
                          rows={5}
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />
                {fields.length > 1 && (
                  <Button
                    variant="ghost"
                    size="icon"
                    type="button"
                    onClick={() => remove(index)}
                    className="cursor-pointer text-colors-text-functional-danger"
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                )}
              </div>
            ))}

            <Button
              type="button"
              variant="outline"
              onClick={() => append(' ')} // "" will cause the inability to add, refer to: https://github.com/orgs/react-hook-form/discussions/8485#discussioncomment-2961861
              className="w-full mt-4"
            >
              <PlusCircle className="mr-2 h-4 w-4" />
              {t('flow.addMessage')}
            </Button>
          </div>
          <FormMessage />
        </FormItem>
      </form>
    </Form>
  );
};

export default MessageForm;
