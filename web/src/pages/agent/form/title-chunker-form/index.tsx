import { FormFieldType, RenderField } from '@/components/dynamic-form';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { BlockButton, Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader } from '@/components/ui/card';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { zodResolver } from '@hookform/resolvers/zod';
import { Trash2 } from 'lucide-react';
import { memo, useEffect, useRef } from 'react';
import { useFieldArray, useForm, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  Hierarchy,
  initialGroupValues,
  initialTitleChunkerValues,
} from '../../constant/pipeline';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';

const outputList = buildOutputList(initialTitleChunkerValues.outputs);

const HierarchyOptions = [
  { label: 'H1', value: Hierarchy.H1 },
  { label: 'H2', value: Hierarchy.H2 },
  { label: 'H3', value: Hierarchy.H3 },
  { label: 'H4', value: Hierarchy.H4 },
  { label: 'H5', value: Hierarchy.H5 },
];

const rulesSchema = z.array(
  z.object({
    levels: z.array(
      z.object({
        expression: z.string().refine(
          (val) => {
            try {
              new RegExp(val);
              return true;
            } catch {
              return false;
            }
          },
          {
            message: 'Must be a valid regular expression string',
          },
        ),
      }),
    ),
  }),
);

export const FormSchema = z.object({
  method: z.enum(['hierarchy', 'tree', 'group']),
  hierarchy: z.string().optional(),
  rules: rulesSchema,
});

export type TitleChunkerFormSchemaType = z.infer<typeof FormSchema>;

type LevelItemProps = {
  index: number;
  parentName: string;
  removeParent: (index: number) => void;
  isLatest: boolean;
};

function LevelItem({
  index,
  parentName,
  isLatest,
  removeParent,
}: LevelItemProps) {
  const { t } = useTranslation();

  const name = `${parentName}.${index}.expression`;

  return (
    <div className="flex items-center">
      <div className="flex-1">
        <RAGFlowFormItem
          name={name}
          label={`${t('flow.regularExpressions')} H${index + 1}`}
          // labelClassName="!hidden"
        >
          <Input className="!m-0" />
        </RAGFlowFormItem>
      </div>
      {isLatest && index > 0 && (
        <Button
          className="self-end"
          type="button"
          variant={'ghost'}
          size="sm"
          onClick={() => removeParent(index)}
        >
          <Trash2 className="h-3 w-3" />
        </Button>
      )}
    </div>
  );
}

type CardBodyProps = {
  cardIndex: number;
  cardName: string;
};

function CardBody({ cardName }: CardBodyProps) {
  const { t } = useTranslation();
  const form = useFormContext();

  const levelsName = `${cardName}.levels`;

  const {
    fields: levelFields,
    append: appendLevel,
    remove: removeLevel,
  } = useFieldArray({
    name: levelsName,
    control: form.control,
  });

  return (
    <CardContent className="p-4">
      <div className="space-y-4">
        {levelFields.map((levelField, levelIndex) => (
          <LevelItem
            key={levelField.id}
            parentName={levelsName}
            index={levelIndex}
            removeParent={removeLevel}
            isLatest={levelIndex === levelFields.length - 1}
          />
        ))}
      </div>
      {levelFields.length < 5 && (
        <BlockButton
          onClick={() => appendLevel({ expression: '' })}
          className="mt-4"
        >
          {t('flow.addLevel', 'Add Level')}
        </BlockButton>
      )}
    </CardContent>
  );
}

type GroupCardBodyProps = {
  cardName: string;
};

function GroupCardBody({ cardName }: GroupCardBodyProps) {
  const { t } = useTranslation();
  const form = useFormContext();

  const levelsName = `${cardName}.levels`;

  const { fields: levelFields } = useFieldArray({
    name: levelsName,
    control: form.control,
  });

  return (
    <CardContent className="p-4">
      <div className="space-y-4">
        {levelFields.map((levelField, levelIndex) => (
          <RAGFlowFormItem
            key={levelField.id}
            name={`${levelsName}.${levelIndex}.expression`}
            label={`${t('flow.regularExpressions')}`}
          >
            <Input />
          </RAGFlowFormItem>
        ))}
      </div>
    </CardContent>
  );
}

const TitleChunkerForm = ({ node }: INextOperatorForm) => {
  const { t } = useTranslation();
  const defaultValues = useFormValues(initialTitleChunkerValues, node);

  const form = useForm<TitleChunkerFormSchemaType>({
    defaultValues,
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
  });
  const isInitialized = useRef(false);
  const initialMode = useRef<string | undefined>(undefined);

  const method = form.watch('method');
  const name = 'rules';

  useEffect(() => {
    if (!isInitialized.current) {
      initialMode.current = method;
      isInitialized.current = true;
      return;
    }

    if (method !== initialMode.current) {
      initialMode.current = method;

      if (method === 'group') {
        form.reset({
          method: 'group',
          hierarchy: undefined,
          rules: initialGroupValues.rules,
        });
      } else {
        form.reset({
          method: method,
          hierarchy: initialTitleChunkerValues.hierarchy,
          rules: initialTitleChunkerValues.rules,
        });
      }
    }
  }, [method, form]);

  const { fields, append, remove } = useFieldArray({
    name: name,
    control: form.control,
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <RenderField
          field={{
            name: 'method',
            type: FormFieldType.Segmented,
            label: '',
            options: [
              { label: t('flow.hierarchy'), value: 'hierarchy' },
              { label: t('flow.tree', 'tree'), value: 'tree' },
              { label: t('flow.group', 'group'), value: 'group' },
            ],
          }}
        />
        {method !== 'group' && (
          <RAGFlowFormItem name={'hierarchy'} label={''}>
            <SelectWithSearch options={HierarchyOptions}></SelectWithSearch>
          </RAGFlowFormItem>
        )}
        {method === 'group' ? (
          <Card>
            <CardHeader className="flex flex-row justify-between items-center py-3 px-4 border-b bg-muted/20">
              <span className="font-medium text-sm">
                {t('flow.rule', 'Rule')} 1
              </span>
            </CardHeader>
            <GroupCardBody cardName={`${name}.0`} />
          </Card>
        ) : (
          <div className="space-y-4">
            {fields.map((cardField, cardIndex) => (
              <Card key={cardField.id}>
                <CardHeader className="flex flex-row justify-between items-center py-3 px-4 border-b bg-muted/20">
                  <div className="flex items-center gap-2">
                    <span className="font-medium text-sm">
                      {t('flow.rule', 'Rule')} {cardIndex + 1}
                    </span>
                  </div>
                  {fields.length > 1 && (
                    <Button
                      type="button"
                      variant={'ghost'}
                      size="sm"
                      onClick={() => remove(cardIndex)}
                      className="h-7 w-7 p-0 text-muted-foreground hover:text-destructive"
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  )}
                </CardHeader>
                <CardBody
                  cardIndex={cardIndex}
                  cardName={`${name}.${cardIndex}`}
                />
              </Card>
            ))}
          </div>
        )}
        <BlockButton
          onClick={() =>
            append({
              levels: [{ expression: '' }],
            })
          }
          disabled={fields.length >= 5}
          className="mt-4"
        >
          {t('flow.rule', 'Add Rule')}
        </BlockButton>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
};

export default memo(TitleChunkerForm);
