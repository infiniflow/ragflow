import { FormContainer } from '@/components/form-container';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { RAGFlowSelect } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { zodResolver } from '@hookform/resolvers/zod';
import { t } from 'i18next';
import { memo, useEffect, useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { INextOperatorForm } from '../../interface';
import { FormWrapper } from '../components/form-wrapper';
import { Output, transferOutputs } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';
import { useValues } from './use-values';
import { useWatchFormChange } from './use-watch-form-change';

function DocGeneratorForm({ node }: INextOperatorForm) {
  const values = useValues(node);

  const FormSchema = z.object({
    output_format: z.string().default('pdf'),
    content: z.string().min(1, 'Content is required'),
    filename: z.string().optional(),
    header_text: z.string().optional(),
    footer_text: z.string().optional(),
    watermark_text: z.string().optional(),
    add_page_numbers: z.boolean(),
    add_timestamp: z.boolean(),
    font_size: z.coerce.number().min(12, 'Font size must be at least 12'),
    outputs: z.object({
      download: z.object({ type: z.string() }),
    }),
  });

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

  const outputFormat = form.watch('output_format');
  const formOutputs = form.watch('outputs');

  const supportsDocumentDecorations =
    outputFormat === 'pdf' || outputFormat === 'docx';

  const supportsTimestamp =
    outputFormat === 'pdf' ||
    outputFormat === 'docx' ||
    outputFormat === 'txt' ||
    outputFormat === 'markdown' ||
    outputFormat === 'html';

  const outputList = useMemo(() => {
    return transferOutputs(formOutputs ?? values.outputs);
  }, [formOutputs, values.outputs]);

  useEffect(() => {
    form.setValue('outputs', values.outputs);
  }, [form, values.outputs]);

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <FormField
            control={form.control}
            name="output_format"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Output Format</FormLabel>
                <FormControl>
                  <RAGFlowSelect
                    {...field}
                    options={[
                      { label: 'PDF', value: 'pdf' },
                      { label: 'DOCX', value: 'docx' },
                      { label: 'TXT', value: 'txt' },
                      { label: 'Markdown', value: 'markdown' },
                      { label: 'HTML', value: 'html' },
                    ]}
                  ></RAGFlowSelect>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="content"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flow.content')}</FormLabel>
                <FormControl>
                  <PromptEditor
                    {...field}
                    showToolbar={true}
                    placeholder="Enter markdown content..."
                  ></PromptEditor>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="filename"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flow.filename')}</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    placeholder="document.ext (auto-generated if empty)"
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          {supportsDocumentDecorations && (
            <>
              <FormField
                control={form.control}
                name="font_size"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('flow.fontSize')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type="number"
                        min={12}
                        onChange={(e) => field.onChange(e.target.value)}
                        onBlur={(e) => {
                          field.onBlur();
                          const value = Number(e.target.value);
                          field.onChange(
                            Number.isFinite(value) && value >= 12 ? value : 12,
                          );
                        }}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="header_text"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Header Text</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder="Header text" />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="footer_text"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Footer Text</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder="Footer text" />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              {outputFormat === 'pdf' && (
                <FormField
                  control={form.control}
                  name="watermark_text"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('flow.watermarkText')}</FormLabel>
                      <FormControl>
                        <Input {...field} placeholder="Watermark text" />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}

              <FormField
                control={form.control}
                name="add_page_numbers"
                render={({ field }) => (
                  <FormItem className="flex flex-row items-center justify-between">
                    <FormLabel>{t('flow.addPageNumbers')}</FormLabel>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
            </>
          )}

          {supportsTimestamp && (
            <FormField
              control={form.control}
              name="add_timestamp"
              render={({ field }) => (
                <FormItem className="flex flex-row items-center justify-between">
                  <FormLabel>{t('flow.addTimestamp')}</FormLabel>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
          )}

          <FormField
            control={form.control}
            name="outputs"
            render={() => <div></div>}
          />
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
}

export default memo(DocGeneratorForm);
