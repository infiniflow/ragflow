import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { zodResolver } from '@hookform/resolvers/zod';
import { PropsWithChildren, useCallback, useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

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
import { useFetchNextKnowledgeListByPage } from '@/hooks/use-knowledge-request';
import { useSelectOwners } from './use-select-owners';

const FormSchema = z.object({
  items: z.array(z.string()).refine((value) => value.some((item) => item), {
    message: 'You have to select at least one item.',
  }),
});

type CheckboxReactHookFormMultipleProps = Pick<
  ReturnType<typeof useFetchNextKnowledgeListByPage>,
  'setOwnerIds' | 'ownerIds'
>;

function CheckboxReactHookFormMultiple({
  setOwnerIds,
  ownerIds,
}: CheckboxReactHookFormMultipleProps) {
  const owners = useSelectOwners();

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      items: [],
    },
  });

  function onSubmit(data: z.infer<typeof FormSchema>) {
    setOwnerIds(data.items);
  }

  const onReset = useCallback(() => {
    setOwnerIds([]);
  }, [setOwnerIds]);

  useEffect(() => {
    form.setValue('items', ownerIds);
  }, [form, ownerIds]);

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className="space-y-8"
        onReset={() => form.reset()}
      >
        <FormField
          control={form.control}
          name="items"
          render={() => (
            <FormItem>
              <div className="mb-4">
                <FormLabel className="text-base">Owner</FormLabel>
              </div>
              {owners.map((item) => (
                <FormField
                  key={item.id}
                  control={form.control}
                  name="items"
                  render={({ field }) => {
                    return (
                      <div className="flex items-center justify-between">
                        <FormItem
                          key={item.id}
                          className="flex flex-row  space-x-3 space-y-0 items-center"
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
                          <FormLabel className="text-lg">
                            {item.label}
                          </FormLabel>
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
        <div className="flex justify-between">
          <Button
            type="button"
            variant={'outline'}
            size={'sm'}
            onClick={onReset}
          >
            Clear
          </Button>
          <Button type="submit" size={'sm'}>
            Submit
          </Button>
        </div>
      </form>
    </Form>
  );
}

export function DatasetsFilterPopover({
  children,
  setOwnerIds,
  ownerIds,
}: PropsWithChildren & CheckboxReactHookFormMultipleProps) {
  return (
    <Popover>
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent>
        <CheckboxReactHookFormMultiple
          setOwnerIds={setOwnerIds}
          ownerIds={ownerIds}
        ></CheckboxReactHookFormMultiple>
      </PopoverContent>
    </Popover>
  );
}
