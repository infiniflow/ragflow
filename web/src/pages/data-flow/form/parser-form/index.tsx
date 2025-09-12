import { FormContainer } from '@/components/form-container';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { BlockButton, Button } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { buildOptions } from '@/utils/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { Trash2 } from 'lucide-react';
import { memo, useCallback } from 'react';
import { useFieldArray, useForm } from 'react-hook-form';
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
      {fields.map((field, index) => {
        const prefix = `${name}.${index}`;
        const fileFormat = form.getValues(`${name}.${index}.fileFormat`);

        const Widget =
          fileFormat && fileFormat in FileFormatWidgetMap
            ? FileFormatWidgetMap[
                fileFormat as keyof typeof FileFormatWidgetMap
              ]
            : OutputFormatFormField;
        return (
          <FormContainer key={field.id}>
            <div className="flex justify-between items-center">
              <span className="text-text-primary text-sm">Parser {index}</span>
              <Button variant={'ghost'} onClick={() => remove(index)}>
                <Trash2 />
              </Button>
            </div>
            <RAGFlowFormItem
              name={buildFieldNameWithPrefix(`fileFormat`, prefix)}
              label="File Formats"
            >
              <SelectWithSearch options={FileFormatOptions}></SelectWithSearch>
            </RAGFlowFormItem>
            <Widget prefix={prefix}></Widget>
          </FormContainer>
        );
      })}
      <BlockButton onClick={add}>Add Parser</BlockButton>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
};

export default memo(ParserForm);
