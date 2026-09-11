import { Card, CardContent, CardHeader } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormMessage,
} from '@/components/ui/form';
import { useGetMcpServer } from '@/hooks/use-mcp-request';
import useGraphStore from '@/pages/agent/store';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { MCPCard } from './mcp-card';
import { useValues } from './use-values';
import { useWatchFormChange } from './use-watch-change';

const FormSchema = z.object({
  items: z.array(z.string()),
});

function MCPForm() {
  const clickedToolId = useGraphStore((state) => state.clickedToolId);
  const values = useValues();
  const form = useForm({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });
  const { data } = useGetMcpServer(clickedToolId);

  useWatchFormChange(form);

  return (
    <Form {...form}>
      <form
        className="space-y-6 p-4"
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <Card className="bg-background-highlight p-5">
          <CardHeader className="p-0 pb-3">
            <div>{data.name}</div>
          </CardHeader>
          <CardContent className="p-0 text-sm">
            <span className="pr-2"> URL:</span>
            <a href={data.url} className="text-accent-primary">
              {data.url}
            </a>
          </CardContent>
        </Card>
        <FormField
          control={form.control}
          name="items"
          render={() => (
            <FormItem className="space-y-2">
              {Object.entries(data.variables?.tools || {}).map(
                ([name, mcp]) => (
                  <FormField
                    key={name}
                    control={form.control}
                    name="items"
                    render={({ field }) => {
                      return (
                        <FormItem
                          key={name}
                          className="flex flex-row items-center gap-2"
                        >
                          <FormControl>
                            <MCPCard key={name} data={{ ...mcp, name }}>
                              <Checkbox
                                className="translate-y-0.5"
                                checked={field.value?.includes(name)}
                                onCheckedChange={(checked) => {
                                  return checked
                                    ? field.onChange([...field.value, name])
                                    : field.onChange(
                                        field.value?.filter(
                                          (value) => value !== name,
                                        ),
                                      );
                                }}
                              />
                            </MCPCard>
                          </FormControl>
                        </FormItem>
                      );
                    }}
                  />
                ),
              )}
              <FormMessage />
            </FormItem>
          )}
        />
      </form>
    </Form>
  );
}

export default memo(MCPForm);
