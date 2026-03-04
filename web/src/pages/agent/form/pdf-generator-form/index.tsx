import { FormContainer } from '@/components/form-container';
import {
  Form,
  FormControl,
  FormDescription,
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
import { memo, useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import {
  PDFGeneratorFontFamily,
  PDFGeneratorLogoPosition,
  PDFGeneratorOrientation,
  PDFGeneratorPageSize,
} from '../../constant';
import { INextOperatorForm } from '../../interface';
import { FormWrapper } from '../components/form-wrapper';
import { Output, transferOutputs } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';
import { useValues } from './use-values';
import { useWatchFormChange } from './use-watch-form-change';

function PDFGeneratorForm({ node }: INextOperatorForm) {
  const values = useValues(node);

  const FormSchema = z.object({
    output_format: z.string().default('pdf'),
    content: z.string().min(1, 'Content is required'),
    title: z.string().optional(),
    subtitle: z.string().optional(),
    header_text: z.string().optional(),
    footer_text: z.string().optional(),
    logo_image: z.string().optional(),
    logo_position: z.string(),
    logo_width: z.number(),
    logo_height: z.number(),
    font_family: z.string(),
    font_size: z.number(),
    title_font_size: z.number(),
    heading1_font_size: z.number(),
    heading2_font_size: z.number(),
    heading3_font_size: z.number(),
    text_color: z.string(),
    title_color: z.string(),
    page_size: z.string(),
    orientation: z.string(),
    margin_top: z.number(),
    margin_bottom: z.number(),
    margin_left: z.number(),
    margin_right: z.number(),
    line_spacing: z.number(),
    filename: z.string().optional(),
    output_directory: z.string(),
    add_page_numbers: z.boolean(),
    add_timestamp: z.boolean(),
    watermark_text: z.string().optional(),
    enable_toc: z.boolean(),
    outputs: z.object({
      file_path: z.object({ type: z.string() }),
      pdf_base64: z.object({ type: z.string() }),
      download: z.object({ type: z.string() }),
      success: z.object({ type: z.string() }),
    }),
  });

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

  const formOutputs = form.watch('outputs');

  const outputList = useMemo(() => {
    return transferOutputs(formOutputs ?? values.outputs);
  }, [formOutputs, values.outputs]);

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          {/* Output Format Selection */}
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
                    ]}
                  ></RAGFlowSelect>
                </FormControl>
                <FormDescription>
                  Choose the output document format
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          {/* Content Section */}
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
                    placeholder="Enter content with markdown formatting...&#10;&#10;**Bold text**, *italic*, # Heading, - List items, etc."
                  ></PromptEditor>
                </FormControl>
                <FormDescription>
                  <div className="text-xs space-y-1">
                    <div>
                      <strong>Markdown support:</strong> **bold**, *italic*,
                      `code`, # Heading 1, ## Heading 2
                    </div>
                    <div>
                      <strong>Lists:</strong> - bullet or 1. numbered
                    </div>
                    <div>
                      <strong>Tables:</strong> | Column 1 | Column 2 | (use | to
                      separate columns, &lt;br&gt; or \n for line breaks in
                      cells)
                    </div>
                    <div>
                      <strong>Other:</strong> --- for horizontal line, ``` for
                      code blocks
                    </div>
                  </div>
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          {/* Title & Subtitle */}
          <FormField
            control={form.control}
            name="title"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flow.title')}</FormLabel>
                <FormControl>
                  <Input {...field} placeholder="Document title (optional)" />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="subtitle"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flow.subtitle')}</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    placeholder="Document subtitle (optional)"
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          {/* Logo Settings */}
          <FormField
            control={form.control}
            name="logo_image"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flow.logoImage')}</FormLabel>
                <FormControl>
                  <div className="space-y-2">
                    <Input
                      type="file"
                      accept="image/*"
                      onChange={(e) => {
                        const file = e.target.files?.[0];
                        if (file) {
                          const reader = new FileReader();
                          reader.onloadend = () => {
                            field.onChange(reader.result as string);
                          };
                          reader.readAsDataURL(file);
                        }
                      }}
                      className="cursor-pointer"
                    />
                    <Input
                      {...field}
                      placeholder="Or paste image path/URL/base64"
                      className="mt-2"
                    />
                  </div>
                </FormControl>
                <FormDescription>
                  Upload an image file or paste a file path/URL/base64
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="logo_position"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flow.logoPosition')}</FormLabel>
                <FormControl>
                  <RAGFlowSelect
                    {...field}
                    options={Object.values(PDFGeneratorLogoPosition).map(
                      (val) => ({ label: val, value: val }),
                    )}
                  ></RAGFlowSelect>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className="grid grid-cols-2 gap-4">
            <FormField
              control={form.control}
              name="logo_width"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('flow.logoWidth')} (inches)</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type="number"
                      step="0.1"
                      onChange={(e) =>
                        field.onChange(parseFloat(e.target.value))
                      }
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="logo_height"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('flow.logoHeight')} (inches)</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type="number"
                      step="0.1"
                      onChange={(e) =>
                        field.onChange(parseFloat(e.target.value))
                      }
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          {/* Font Settings */}
          <FormField
            control={form.control}
            name="font_family"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flow.fontFamily')}</FormLabel>
                <FormControl>
                  <RAGFlowSelect
                    {...field}
                    options={Object.values(PDFGeneratorFontFamily).map(
                      (val) => ({ label: val, value: val }),
                    )}
                  ></RAGFlowSelect>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className="grid grid-cols-2 gap-4">
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
                      onChange={(e) => field.onChange(parseInt(e.target.value))}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="title_font_size"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('flow.titleFontSize')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type="number"
                      onChange={(e) => field.onChange(parseInt(e.target.value))}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          {/* Page Settings */}
          <FormField
            control={form.control}
            name="page_size"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flow.pageSize')}</FormLabel>
                <FormControl>
                  <RAGFlowSelect
                    {...field}
                    options={Object.values(PDFGeneratorPageSize).map((val) => ({
                      label: val,
                      value: val,
                    }))}
                  ></RAGFlowSelect>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="orientation"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flow.orientation')}</FormLabel>
                <FormControl>
                  <RAGFlowSelect
                    {...field}
                    options={Object.values(PDFGeneratorOrientation).map(
                      (val) => ({ label: val, value: val }),
                    )}
                  ></RAGFlowSelect>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          {/* Margins */}
          <div className="grid grid-cols-2 gap-4">
            <FormField
              control={form.control}
              name="margin_top"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('flow.marginTop')} (inches)</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type="number"
                      step="0.1"
                      onChange={(e) =>
                        field.onChange(parseFloat(e.target.value))
                      }
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="margin_bottom"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('flow.marginBottom')} (inches)</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type="number"
                      step="0.1"
                      onChange={(e) =>
                        field.onChange(parseFloat(e.target.value))
                      }
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          {/* Output Settings */}
          <FormField
            control={form.control}
            name="filename"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flow.filename')}</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    placeholder="document.pdf (auto-generated if empty)"
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="output_directory"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flow.outputDirectory')}</FormLabel>
                <FormControl>
                  <Input {...field} placeholder="/tmp/pdf_outputs" />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          {/* Additional Options */}
          <FormField
            control={form.control}
            name="add_page_numbers"
            render={({ field }) => (
              <FormItem className="flex flex-row items-center justify-between rounded-lg border p-3">
                <div className="space-y-0.5">
                  <FormLabel>{t('flow.addPageNumbers')}</FormLabel>
                  <FormDescription>
                    Add page numbers to the document
                  </FormDescription>
                </div>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="add_timestamp"
            render={({ field }) => (
              <FormItem className="flex flex-row items-center justify-between rounded-lg border p-3">
                <div className="space-y-0.5">
                  <FormLabel>{t('flow.addTimestamp')}</FormLabel>
                  <FormDescription>
                    Add generation timestamp to the document
                  </FormDescription>
                </div>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="watermark_text"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flow.watermarkText')}</FormLabel>
                <FormControl>
                  <Input {...field} placeholder="Watermark text (optional)" />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

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

export default memo(PDFGeneratorForm);
