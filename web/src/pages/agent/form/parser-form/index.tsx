import { Collapse } from '@/components/collapse';
import { useSyncExternalFormErrors } from '@/components/pipeline-operator-tabs/use-sync-external-form-errors';
import { BlockButton, Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Form } from '@/components/ui/form';
import { useFetchDefaultModelDictionary } from '@/hooks/use-llm-request';
import { zodResolver } from '@hookform/resolvers/zod';
import { LucideTrash2 } from 'lucide-react';
import { memo, useCallback, useMemo } from 'react';
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
import { FormSchema, ParserFormSchemaType } from './schema';
import { SpreadsheetFormFields } from './spreadsheet-form-fields';
import {
  HtmlFormFields,
  TextMarkdownFormFields,
} from './text-html-form-fields';
import { buildInitialParserSetup } from './utils';
import { AudioFormFields, VideoFormFields } from './video-form-fields';
import { WordFormFields } from './word-form-fields';

export { FormSchema } from './schema';
export type { ParserFormSchemaType } from './schema';

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
  canRemove: boolean;
  onRemove: (index: number) => void;
};

function ParserItem({ name, index, canRemove, onRemove }: ParserItemProps) {
  const { t } = useTranslation();
  const form = useFormContext<ParserFormSchemaType>();

  const prefix = `${name}.${index}`;
  const fileFormat = form.watch(`setups.${index}.fileFormat`);

  const Widget =
    typeof fileFormat === 'string' && fileFormat in FileFormatWidgetMap
      ? FileFormatWidgetMap[fileFormat as keyof typeof FileFormatWidgetMap]
      : () => <></>;

  const handleRemove = useCallback(() => {
    onRemove(index);
  }, [onRemove, index]);

  return (
    <Card as="section" className="bg-bg-card px-5 py-2.5 border-none">
      {/* The file format has no visible input; keep it registered so it is
          still submitted with the form. */}
      <input type="hidden" {...form.register(`setups.${index}.fileFormat`)} />
      <Collapse
        title={t(`flow.fileFormatOptions.${fileFormat}`)}
        defaultOpen
        rightContent={
          canRemove ? (
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              onClick={handleRemove}
            >
              <LucideTrash2 className="size-4" />
            </Button>
          ) : undefined
        }
      >
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

type ParserFormProps = INextOperatorForm & {
  // Dataset-side embeddings (settings page, document pipeline dialog) show a
  // fixed set of file types; only the canvas parser allows add/remove.
  fixedFileFormats?: boolean;
};

type AddFileTypeMenuItemProps = {
  fileType: FileType;
  onSelect: (fileType: FileType) => void;
};

function AddFileTypeMenuItem({ fileType, onSelect }: AddFileTypeMenuItemProps) {
  const { t } = useTranslation();
  const handleSelect = useCallback(() => {
    onSelect(fileType);
  }, [onSelect, fileType]);

  return (
    <DropdownMenuItem onSelect={handleSelect}>
      {t(`flow.fileFormatOptions.${fileType}`)}
    </DropdownMenuItem>
  );
}

const ParserForm = ({
  node,
  onValuesChange,
  hideOutputs,
  externalErrors,
  fixedFileFormats,
}: ParserFormProps) => {
  const { t } = useTranslation();
  const defaultModelDictionary = useFetchDefaultModelDictionary();
  // Show the saved values as-is: an empty llm_id means the user cleared the
  // model, and the backend falls back to the tenant default at parse time.
  // Prefilling it here would make a cleared model reappear and be written back.
  const defaultValues = useFormValues(initialParserValues, node);

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues,
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
  });

  useSyncExternalFormErrors(form, externalErrors);

  const name = 'setups';
  const { fields, append, remove } = useFieldArray({
    name,
    control: form.control,
  });

  useWatchFormChange(node?.id, form);
  useFormChangeCallback(form, onValuesChange);

  const remainingFileTypes = useMemo(() => {
    const presentFileTypes = new Set(fields.map((x) => x.fileFormat));
    return initialParserValues.setups
      .map((x) => x.fileFormat)
      .filter(
        (fileType): fileType is FileType =>
          !!fileType && !presentFileTypes.has(fileType),
      );
  }, [fields]);

  const handleAddFileType = useCallback(
    (fileType: FileType) => {
      const setup = buildInitialParserSetup(fileType, defaultModelDictionary);
      if (setup) {
        append(setup);
      }
    },
    [append, defaultModelDictionary],
  );

  const canRemove = !fixedFileFormats && fields.length > 1;

  return (
    <Form {...form}>
      <form className="space-y-5 px-5">
        {fields.map((field, index) => {
          return (
            <ParserItem
              key={field.id}
              name={name}
              index={index}
              canRemove={canRemove}
              onRemove={remove}
            ></ParserItem>
          );
        })}
        {!fixedFileFormats && remainingFileTypes.length > 0 && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <BlockButton type="button">{t('flow.addFileType')}</BlockButton>
            </DropdownMenuTrigger>
            <DropdownMenuContent>
              {remainingFileTypes.map((fileType) => (
                <AddFileTypeMenuItem
                  key={fileType}
                  fileType={fileType}
                  onSelect={handleAddFileType}
                />
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        )}
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
