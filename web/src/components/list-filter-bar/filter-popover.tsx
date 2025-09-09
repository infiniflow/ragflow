import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { zodResolver } from '@hookform/resolvers/zod';
import { PropsWithChildren, useCallback, useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { ZodArray, ZodString, z } from 'zod';

import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { t } from 'i18next';
import { FilterChange, FilterCollection, FilterValue } from './interface';

export type CheckboxFormMultipleProps = {
  filters?: FilterCollection[];
  value?: FilterValue;
  onChange?: FilterChange;
  onOpenChange?: (open: boolean) => void;
  setOpen(open: boolean): void;
};

function CheckboxFormMultiple({
  filters = [],
  value,
  onChange,
  setOpen,
}: CheckboxFormMultipleProps) {
  const fieldsDict = filters?.reduce<Record<string, Array<any>>>((pre, cur) => {
    pre[cur.field] = [];
    return pre;
  }, {});

  const FormSchema = z.object(
    filters.reduce<Record<string, ZodArray<ZodString, 'many'>>>((pre, cur) => {
      pre[cur.field] = z.array(z.string());

      // .refine((value) => value.some((item) => item), {
      //   message: 'You have to select at least one item.',
      // });
      return pre;
    }, {}),
  );

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: fieldsDict,
  });

  function onSubmit(data: z.infer<typeof FormSchema>) {
    onChange?.(data);
    setOpen(false);
  }

  const onReset = useCallback(() => {
    onChange?.(fieldsDict);
    setOpen(false);
  }, [fieldsDict, onChange, setOpen]);

  useEffect(() => {
    form.reset(value);
  }, [form, value]);

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className="space-y-8 px-5 py-2.5"
        onReset={() => form.reset()}
      >
        {filters.map((x) => (
          <FormField
            key={x.field}
            control={form.control}
            name={x.field}
            render={() => (
              <FormItem className="space-y-4">
                <div>
                  <FormLabel className="text-base text-text-sub-title-invert">
                    {x.label}
                  </FormLabel>
                </div>
                {x.list.map((item) => (
                  <FormField
                    key={item.id}
                    control={form.control}
                    name={x.field}
                    render={({ field }) => {
                      return (
                        <div className="flex items-center justify-between text-text-primary text-xs">
                          <FormItem
                            key={item.id}
                            className="flex flex-row  space-x-3 space-y-0 items-center "
                          >
                            <FormControl>
                              <Checkbox
                                checked={field.value?.includes(item.id)}
                                onCheckedChange={(checked) => {
                                  return checked
                                    ? field.onChange([...field.value, item.id])
                                    : field.onChange(
                                        field.value?.filter(
                                          (value) => value !== item.id,
                                        ),
                                      );
                                }}
                              />
                            </FormControl>
                            <FormLabel>{item.label}</FormLabel>
                          </FormItem>
                          <span className=" text-sm">{item.count}</span>
                        </div>
                      );
                    }}
                  />
                ))}
                <FormMessage />
              </FormItem>
            )}
          />
        ))}
        <div className="flex justify-end gap-5">
          <Button
            type="button"
            variant={'outline'}
            size={'sm'}
            onClick={onReset}
          >
            {t('common.clear')}
          </Button>
          <Button type="submit" size={'sm'}>
            {t('common.submit')}
          </Button>
        </div>
      </form>
    </Form>
  );
}

export function FilterPopover({
  children,
  value,
  onChange,
  onOpenChange,
  filters,
}: PropsWithChildren & Omit<CheckboxFormMultipleProps, 'setOpen'>) {
  const [open, setOpen] = useState(false);
  const onOpenChangeFun = useCallback(
    (e: boolean) => {
      onOpenChange?.(e);
      setOpen(e);
    },
    [onOpenChange],
  );
  return (
    <Popover open={open} onOpenChange={onOpenChangeFun}>
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent className="p-0">
        <CheckboxFormMultiple
          onChange={onChange}
          value={value}
          filters={filters}
          setOpen={setOpen}
        ></CheckboxFormMultiple>
      </PopoverContent>
    </Popover>
  );
}
