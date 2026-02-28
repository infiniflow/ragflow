import {
  SelectWithSearch,
  SelectWithSearchFlagOptionType,
} from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { BlockButton, Button } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { MultiSelect } from '@/components/ui/multi-select';
import { Separator } from '@/components/ui/separator';
import { cn } from '@/lib/utils';
import { buildOptions } from '@/utils/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useHover } from 'ahooks';
import { Trash2 } from 'lucide-react';
import { memo, useCallback, useEffect, useMemo, useRef } from 'react';
import {
  useFieldArray,
  UseFieldArrayRemove,
  useForm,
  useFormContext,
} from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  FileType,
  InitialOutputFormatMap,
  initialParserValues,
  MAIN_CONTENT_PREPROCESS_VALUE,
  PreprocessValue,
} from '../../constant/pipeline';
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
import { buildFieldNameWithPrefix } from './utils';
import { AudioFormFields, VideoFormFields } from './video-form-fields';

const outputList = buildOutputList(initialParserValues.outputs);

type PreprocessOptionConfig = {
  value: PreprocessValue;
  required?: boolean;
};

const DefaultPreprocessOptionConfigs: PreprocessOptionConfig[] = [
  { value: MAIN_CONTENT_PREPROCESS_VALUE, required: true },
];

const PreprocessOptionConfigsMap: Partial<
  Record<FileType, PreprocessOptionConfig[]>
> = {
  [FileType.PDF]: [
    { value: MAIN_CONTENT_PREPROCESS_VALUE, required: true },
    { value: PreprocessValue.abstract },
    { value: PreprocessValue.author },
    { value: PreprocessValue.section_title },
  ],
  [FileType.PowerPoint]: [
    { value: MAIN_CONTENT_PREPROCESS_VALUE, required: true },
  ],
  [FileType.Spreadsheet]: [
    { value: MAIN_CONTENT_PREPROCESS_VALUE, required: true },
  ],
  [FileType.TextMarkdown]: [
    { value: MAIN_CONTENT_PREPROCESS_VALUE, required: true },
    { value: PreprocessValue.section_title },
  ],
  [FileType.Docx]: [
    { value: MAIN_CONTENT_PREPROCESS_VALUE, required: true },
    { value: PreprocessValue.section_title },
  ],
};

function getPreprocessOptionConfigs(fileType?: FileType) {
  if (!fileType) {
    return DefaultPreprocessOptionConfigs;
  }

  return PreprocessOptionConfigsMap[fileType] ?? DefaultPreprocessOptionConfigs;
}

function normalizePreprocessValuesByFileType(
  fileType: FileType | undefined,
  values: string[] | undefined,
) {
  const optionConfigs = getPreprocessOptionConfigs(fileType);
  const allowedValueSet = new Set(optionConfigs.map((x) => x.value));
  const requiredValues = optionConfigs
    .filter((x) => x.required)
    .map((x) => x.value);
  const normalizedOptionalValues = (Array.isArray(values) ? values : []).filter(
    (value) => allowedValueSet.has(value as PreprocessValue),
  ) as PreprocessValue[];

  return Array.from(
    new Set<PreprocessValue>([...requiredValues, ...normalizedOptionalValues]),
  );
}

function isSameStringArray(a: string[] | undefined, b: string[]) {
  if (!a || a.length !== b.length) {
    return false;
  }

  return a.every((item, idx) => item === b[idx]);
}

const FileFormatWidgetMap = {
  [FileType.PDF]: PdfFormFields,
  [FileType.Spreadsheet]: SpreadsheetFormFields,
  [FileType.PowerPoint]: PptFormFields,
  [FileType.Video]: VideoFormFields,
  [FileType.Audio]: AudioFormFields,
  [FileType.Email]: EmailFormFields,
  [FileType.Image]: ImageFormFields,
};

type ParserItemProps = {
  name: string;
  index: number;
  fieldLength: number;
  remove: UseFieldArrayRemove;
  fileFormatOptions: SelectWithSearchFlagOptionType[];
};

export const FormSchema = z.object({
  setups: z.array(
    z.object({
      fileFormat: z.string().nullish(),
      preprocess: z.array(z.string()).optional(),
      output_format: z.string().optional(),
      parse_method: z.string().optional(),
      lang: z.string().optional(),
      fields: z.array(z.string()).optional(),
      llm_id: z.string().optional(),
      system_prompt: z.string().optional(),
      table_result_type: z.string().optional(),
      markdown_image_response_type: z.string().optional(),
    }),
  ),
});

export type ParserFormSchemaType = z.infer<typeof FormSchema>;

function ParserItem({
  name,
  index,
  fieldLength,
  remove,
  fileFormatOptions,
}: ParserItemProps) {
  const { t } = useTranslation();
  const form = useFormContext<ParserFormSchemaType>();
  const ref = useRef(null);
  const isHovering = useHover(ref);

  const prefix = `${name}.${index}`;
  const fileFormat = form.getValues(`setups.${index}.fileFormat`);

  const values = form.getValues();
  const parserList = values.setups.slice(); // Adding, deleting, or modifying the parser array will not change the reference.

  const filteredFileFormatOptions = useMemo(() => {
    const otherFileFormatList = parserList
      .filter((_, idx) => idx !== index)
      .map((x) => x.fileFormat);

    return fileFormatOptions.filter((x) => {
      return !otherFileFormatList.includes(x.value);
    });
  }, [fileFormatOptions, index, parserList]);

  const Widget =
    typeof fileFormat === 'string' && fileFormat in FileFormatWidgetMap
      ? FileFormatWidgetMap[fileFormat as keyof typeof FileFormatWidgetMap]
      : () => <></>;

  const handleFileTypeChange = useCallback(
    (value: FileType) => {
      form.setValue(
        `setups.${index}.output_format`,
        InitialOutputFormatMap[value],
        { shouldDirty: true, shouldValidate: true, shouldTouch: true },
      );
    },
    [form, index],
  );

  const handlePreprocessChange = useCallback(
    (value: PreprocessValue[]) => {
      form.setValue(`setups.${index}.preprocess`, value, {
        shouldDirty: true,
        shouldValidate: true,
        shouldTouch: true,
      });
    },
    [form, index],
  );

  const preprocessOptions = useMemo(() => {
    const optionConfigs = getPreprocessOptionConfigs(fileFormat as FileType);

    return optionConfigs.map((optionConfig) => {
      const labelMap: Record<string, string> = {
        [MAIN_CONTENT_PREPROCESS_VALUE]: t('flow.preprocess.mainContent'),
        [PreprocessValue.section_title]: t('flow.preprocess.sectionTitle'),
        [PreprocessValue.abstract]: t('flow.preprocess.abstract'),
        [PreprocessValue.author]: t('flow.preprocess.author'),
      };

      const label = labelMap[optionConfig.value] || optionConfig.value;

      return {
        value: optionConfig.value,
        disabled: optionConfig.required,
        label: label,
      };
    });
  }, [fileFormat, t]);

  useEffect(() => {
    const currentPreprocessValues = form.getValues(
      `setups.${index}.preprocess`,
    ) as string[] | undefined;
    const normalizedPreprocessValues = normalizePreprocessValuesByFileType(
      fileFormat as FileType,
      currentPreprocessValues,
    );

    if (
      !isSameStringArray(currentPreprocessValues, normalizedPreprocessValues)
    ) {
      form.setValue(`setups.${index}.preprocess`, normalizedPreprocessValues, {
        shouldDirty: false,
        shouldValidate: true,
      });
    }
  }, [fileFormat, form, index]);

  return (
    <section
      className={cn('space-y-5 py-2.5 rounded-md', {
        'bg-state-error-5': isHovering,
      })}
    >
      <div className="flex justify-between items-center">
        <span className="text-text-primary text-sm font-medium">
          Parser {index + 1}
        </span>
        {index > 0 && (
          <Button variant={'ghost'} onClick={() => remove(index)} ref={ref}>
            <Trash2 />
          </Button>
        )}
      </div>
      <RAGFlowFormItem
        name={buildFieldNameWithPrefix(`fileFormat`, prefix)}
        label={t('flow.fileFormats')}
      >
        {(field) => (
          <SelectWithSearch
            value={field.value}
            onChange={(val) => {
              field.onChange(val);
              handleFileTypeChange(val as FileType);
            }}
            options={filteredFileFormatOptions}
          ></SelectWithSearch>
        )}
      </RAGFlowFormItem>
      <Widget prefix={prefix} fileType={fileFormat as FileType}></Widget>
      <div className="hidden">
        <OutputFormatFormField
          prefix={prefix}
          fileType={fileFormat as FileType}
        />
      </div>
      <RAGFlowFormItem
        name={buildFieldNameWithPrefix(`preprocess`, prefix)}
        label={t('flow.preprocess.preprocess')}
      >
        {(field) => (
          <MultiSelect
            value={field.value || []}
            onValueChange={(val) => {
              const nextValues = normalizePreprocessValuesByFileType(
                fileFormat as FileType,
                val,
              );
              field.onChange(nextValues);
              handlePreprocessChange(nextValues);
            }}
            showSelectAll={false}
            options={preprocessOptions}
          ></MultiSelect>
        )}
      </RAGFlowFormItem>
      {index < fieldLength - 1 && <Separator />}
    </section>
  );
}

const ParserForm = ({ node }: INextOperatorForm) => {
  const { t } = useTranslation();
  const defaultValues = useFormValues(initialParserValues, node);

  const FileFormatOptions = buildOptions(FileType, t, 'flow.fileFormatOptions');

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues,
    resolver: zodResolver(FormSchema),
    shouldUnregister: true,
  });

  const name = 'setups';
  const { fields, remove, append } = useFieldArray({
    name,
    control: form.control,
  });

  const add = useCallback(() => {
    append({
      fileFormat: null,
      output_format: '',
      parse_method: '',
      lang: '',
      fields: [],
      llm_id: '',
      table_result_type: '',
      markdown_image_response_type: '',
      preprocess: [],
    });
  }, [append]);

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <form className="px-5">
        {fields.map((field, index) => {
          return (
            <ParserItem
              key={field.id}
              name={name}
              index={index}
              fieldLength={fields.length}
              remove={remove}
              fileFormatOptions={FileFormatOptions}
            ></ParserItem>
          );
        })}
        {fields.length < FileFormatOptions.length && (
          <BlockButton onClick={add} type="button" className="mt-2.5">
            {t('flow.addParser')}
          </BlockButton>
        )}
      </form>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
};

export default memo(ParserForm);
