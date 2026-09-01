import { Collapse } from '@/components/collapse';
import { useSyncExternalFormErrors } from '@/components/pipeline-operator-tabs/use-sync-external-form-errors';
import { Card } from '@/components/ui/card';
import { Form } from '@/components/ui/form';
import { useFetchDefaultModelDictionary } from '@/hooks/use-llm-request';
import i18n from '@/locales/config';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useMemo } from 'react';
import { useFieldArray, useForm, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { FileType, initialParserValues } from '../../constant/pipeline';
import { useFormChangeCallback } from '../../hooks/use-form-change-callback';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { Output } from '../components/output';
import { OutputFormatFormField } from './common-form-fields';
import { EmailFormFields } from './email-form-fields';
import { ImageFormFields } from './image-form-fields';
import { PdfFormFields } from './pdf-form-fields';
import { PptFormFields } from './ppt-form-fields';
import { SpreadsheetFormFields } from './spreadsheet-form-fields';
import {
  HtmlFormFields,
  TextMarkdownFormFields,
} from './text-html-form-fields';
import { withDefaultParserModels } from './utils';
import { AudioFormFields, VideoFormFields } from './video-form-fields';
import { WordFormFields } from './word-form-fields';

const outputList = buildOutputList(initialParserValues.outputs);

const FileFormatWidgetMap = {
  [FileType.PDF]: PdfFormFields,
  [FileType.Spreadsheet]: SpreadsheetFormFields,
  [FileType.PowerPoint]: PptFormFields,
  [FileType.Doc]: WordFormFields,
  [FileType.Docx]: WordFormFields,
  [FileType.Video]: VideoFormFields,
  [FileType.Audio]: AudioFormFields,
  [FileType.Email]: EmailFormFields,
  [FileType.Image]: ImageFormFields,
  [FileType.TextMarkdown]: TextMarkdownFormFields,
  [FileType.Html]: HtmlFormFields,
};

type ParserItemProps = {
  name: string;
  index: number;
};

const SetupSchema = z
  .object({
    fileFormat: z.string().nullish(),
    // preprocess: z.array(z.string()).optional(),
    output_format: z.string().optional(),
    parse_method: z.string().optional(),
    lang: z.string().optional(),
    fields: z.array(z.string()).optional(),
    vlm: z.object({ llm_id: z.string().optional() }).optional(),
    flatten_media_to_text: z.boolean().optional(),
    system_prompt: z.string().optional(),
    table_result_type: z.string().optional(),
    markdown_image_response_type: z.string().optional(),
    enable_multi_column: z.boolean().optional(),
    remove_toc: z.boolean().optional(),
    remove_header_footer: z.boolean().optional(),
    pages: z
      .array(
        z
          .object({
            // Keep these checks on the fields themselves: an object-level
            // `superRefine` is skipped whenever the base shape fails to
            // parse, so one missing sibling would silently swallow the
            // other field's error.
            from: z.coerce
              .number()
              .int(i18n.t('knowledgeDetails.pageRangeFromInvalid'))
              .min(1, i18n.t('knowledgeDetails.pageRangeFromInvalid')),
            to: z.coerce
              .number()
              .int(i18n.t('knowledgeDetails.pageRangeToInvalid'))
              .min(1, i18n.t('knowledgeDetails.pageRangeToInvalid')),
          })
          .refine(({ from, to }) => to >= from, {
            path: ['to'],
            message: i18n.t('knowledgeDetails.pageRangeToInvalid'),
          }),
      )
      .optional(),
  })
  .superRefine((values, ctx) => {
    if (values.fileFormat === FileType.Email && !values.fields?.length) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['fields'],
        message: 'Fields is required',
      });
    }
    if (
      (values.fileFormat === FileType.Video ||
        values.fileFormat === FileType.Audio) &&
      !values.vlm?.llm_id
    ) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['vlm', 'llm_id'],
        message: 'Model is required',
      });
    }
  });

export const FormSchema = z.object({
  setups: z.array(SetupSchema),
});

export type ParserFormSchemaType = z.infer<typeof FormSchema>;

function ParserItem({ name, index }: ParserItemProps) {
  const { t } = useTranslation();
  const form = useFormContext<ParserFormSchemaType>();

  const prefix = `${name}.${index}`;
  const fileFormat = form.watch(`setups.${index}.fileFormat`);

  const Widget =
    typeof fileFormat === 'string' && fileFormat in FileFormatWidgetMap
      ? FileFormatWidgetMap[fileFormat as keyof typeof FileFormatWidgetMap]
      : () => <></>;

  return (
    <Card as="section" className="bg-bg-card px-5 py-2.5 border-none">
      {/* The file format is fixed per parser item; keep it registered so it is
          still submitted with the form. */}
      <input type="hidden" {...form.register(`setups.${index}.fileFormat`)} />
      <Collapse title={t(`flow.fileFormatOptions.${fileFormat}`)} defaultOpen>
        <div className="space-y-5">
          <Widget prefix={prefix} fileType={fileFormat as FileType}></Widget>
        </div>
      </Collapse>
      <div className="hidden">
        <OutputFormatFormField
          prefix={prefix}
          fileType={fileFormat as FileType}
        />
      </div>
    </Card>
  );
}

const ParserForm = ({
  node,
  onValuesChange,
  hideOutputs,
  externalErrors,
}: INextOperatorForm) => {
  const defaultModelDictionary = useFetchDefaultModelDictionary();
  const formValues = useFormValues(initialParserValues, node);
  const defaultValues = useMemo(
    () => withDefaultParserModels(formValues, defaultModelDictionary),
    [formValues, defaultModelDictionary],
  );

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues,
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
  });

  useSyncExternalFormErrors(form, externalErrors);

  const name = 'setups';
  const { fields } = useFieldArray({
    name,
    control: form.control,
  });

  useWatchFormChange(node?.id, form);
  useFormChangeCallback(form, onValuesChange);

  return (
    <Form {...form}>
      <form className="space-y-5 px-5">
        {fields.map((field, index) => {
          return (
            <ParserItem key={field.id} name={name} index={index}></ParserItem>
          );
        })}
      </form>
      {!hideOutputs && (
        <div className="p-5">
          <Output list={outputList}></Output>
        </div>
      )}
    </Form>
  );
};

export default memo(ParserForm);
