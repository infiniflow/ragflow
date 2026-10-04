import { RAGFlowFormItem } from '@/components/ragflow-form';
import { FormContainer } from '@/components/form-container';
import { Form } from '@/components/ui/form';
import { Input, NumberInput } from '@/components/ui/input';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { SwitchFormField } from '@/components/switch-form-field';
import { zodResolver } from '@hookform/resolvers/zod';
import i18n from '@/locales/config';
import { memo, useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  DocGeneratorFormatFeatures,
  DocGeneratorFormatOptions,
  DocGeneratorOutputFormat,
} from '../../constant/doc-generator';
import { INextOperatorForm } from '../../interface';
import { FormWrapper } from '../components/form-wrapper';
import { Output, transferOutputs } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';
import { useValues } from './use-values';
import { useWatchFormChange } from './use-watch-form-change';

const FormSchema = z.object({
  output_format: z.nativeEnum(DocGeneratorOutputFormat),
  content: z.string().min(1, i18n.t('flow.contentRequired')),
  filename: z.string().optional(),
  header_text: z.string().optional(),
  footer_text: z.string().optional(),
  watermark_text: z.string().optional(),
  add_page_numbers: z.boolean(),
  add_timestamp: z.boolean(),
  include_download_info_in_content: z.boolean(),
  font_size: z.coerce.number().min(12, i18n.t('flow.fontSizeMin')),
  outputs: z.object({
    doc_id: z.object({ type: z.string() }),
    filename: z.object({ type: z.string() }),
    mime_type: z.object({ type: z.string() }),
    size: z.object({ type: z.string() }),
    download: z.object({ type: z.string() }),
  }),
});

function DocGeneratorForm({ node }: INextOperatorForm) {
  const { t } = useTranslation();
  const values = useValues(node);

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

  const outputFormat = form.watch('output_format');

  const formatFeatures = DocGeneratorFormatFeatures[outputFormat];
  const supportsDocumentDecorations = formatFeatures.decorations;
  const supportsTimestamp = formatFeatures.timestamp;

  const outputList = useMemo(() => {
    return transferOutputs(values.outputs);
  }, [values.outputs]);

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <RAGFlowFormItem label={t('flow.outputFormat')} name="output_format">
            <SelectWithSearch
              options={DocGeneratorFormatOptions}
            ></SelectWithSearch>
          </RAGFlowFormItem>

          <RAGFlowFormItem label={t('flow.content')} name="content">
            <PromptEditor
              showToolbar={true}
              placeholder={t('flow.contentPlaceholder')}
            ></PromptEditor>
          </RAGFlowFormItem>

          <SwitchFormField
            vertical={false}
            label={t('flow.includeDownloadInfoInContent')}
            name="include_download_info_in_content"
          />

          <RAGFlowFormItem label={t('flow.filename')} name="filename">
            <Input placeholder={t('flow.filenamePlaceholder')}></Input>
          </RAGFlowFormItem>

          {supportsDocumentDecorations && (
            <>
              <RAGFlowFormItem label={t('flow.fontSize')} name="font_size">
                {(field) => <NumberInput min={12} {...field}></NumberInput>}
              </RAGFlowFormItem>

              <RAGFlowFormItem label={t('flow.headerText')} name="header_text">
                <Input placeholder={t('flow.headerText')}></Input>
              </RAGFlowFormItem>

              <RAGFlowFormItem label={t('flow.footerText')} name="footer_text">
                <Input placeholder={t('flow.footerText')}></Input>
              </RAGFlowFormItem>

              {outputFormat === DocGeneratorOutputFormat.Pdf && (
                <RAGFlowFormItem
                  label={t('flow.watermarkText')}
                  name="watermark_text"
                >
                  <Input placeholder={t('flow.watermarkText')}></Input>
                </RAGFlowFormItem>
              )}

              <SwitchFormField
                vertical={false}
                label={t('flow.addPageNumbers')}
                name="add_page_numbers"
              />
            </>
          )}

          {supportsTimestamp && (
            <SwitchFormField
              vertical={false}
              label={t('flow.addTimestamp')}
              name="add_timestamp"
            />
          )}
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList} isFormRequired></Output>
      </div>
    </Form>
  );
}

export default memo(DocGeneratorForm);
