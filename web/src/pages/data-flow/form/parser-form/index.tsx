import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { BlockButton, Button } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { Separator } from '@/components/ui/separator';
import { cn } from '@/lib/utils';
import { buildOptions } from '@/utils/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useHover } from 'ahooks';
import { Trash2 } from 'lucide-react';
import { memo, useCallback, useRef } from 'react';
import {
  UseFieldArrayRemove,
  useFieldArray,
  useForm,
  useFormContext,
} from 'react-hook-form';
import { z } from 'zod';
import { FileType, initialParserValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { Output } from '../components/output';
import { OutputFormatFormField } from './common-form-fields';
import { EmailFormFields } from './email-form-fields';
import { ImageFormFields } from './image-form-fields';
import { PdfFormFields } from './pdf-form-fields';
import { buildFieldNameWithPrefix } from './utils';
import { VideoFormFields } from './video-form-fields';

const outputList = buildOutputList(initialParserValues.outputs);

const FileFormatOptions = buildOptions(FileType);

const FileFormatWidgetMap = {
  [FileType.PDF]: PdfFormFields,
  [FileType.Video]: VideoFormFields,
  [FileType.Audio]: VideoFormFields,
  [FileType.Email]: EmailFormFields,
  [FileType.Image]: ImageFormFields,
};

type ParserItemProps = {
  name: string;
  index: number;
  fieldLength: number;
  remove: UseFieldArrayRemove;
};

function ParserItem({ name, index, fieldLength, remove }: ParserItemProps) {
  const form = useFormContext();
  const ref = useRef(null);
  const isHovering = useHover(ref);

  const prefix = `${name}.${index}`;
  const fileFormat = form.getValues(`${name}.${index}.fileFormat`);

  const Widget =
    fileFormat && fileFormat in FileFormatWidgetMap
      ? FileFormatWidgetMap[fileFormat as keyof typeof FileFormatWidgetMap]
      : OutputFormatFormField;
  return (
    <section
      className={cn('space-y-5 p-5 rounded-md', {
        'bg-state-error-5': isHovering,
      })}
    >
      <div className="flex justify-between items-center">
        <span className="text-text-primary text-sm">Parser {index}</span>
        {index > 0 && (
          <Button variant={'ghost'} onClick={() => remove(index)} ref={ref}>
            <Trash2 />
          </Button>
        )}
      </div>
      <RAGFlowFormItem
        name={buildFieldNameWithPrefix(`fileFormat`, prefix)}
        label="File Formats"
      >
        <SelectWithSearch options={FileFormatOptions}></SelectWithSearch>
      </RAGFlowFormItem>
      <Widget prefix={prefix} fileType={fileFormat as FileType}></Widget>
      {index < fieldLength - 1 && <Separator />}
    </section>
  );
}

export const FormSchema = z.object({
  parser: z.array(
    z.object({
      fileFormat: z.string().optional(),
    }),
  ),
});

const ParserForm = ({ node }: INextOperatorForm) => {
  const defaultValues = useFormValues(initialParserValues, node);

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues,
    resolver: zodResolver(FormSchema),
    shouldUnregister: true,
  });

  const name = 'parser';
  const { fields, remove, append } = useFieldArray({
    name,
    control: form.control,
  });

  const add = useCallback(() => {
    append({
      fileFormat: undefined,
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
            ></ParserItem>
          );
        })}
        <BlockButton onClick={add} type="button">
          Add Parser
        </BlockButton>
      </form>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
};

export default memo(ParserForm);
