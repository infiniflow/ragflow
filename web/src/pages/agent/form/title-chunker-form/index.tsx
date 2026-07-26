import { FormFieldType, RenderField } from '@/components/dynamic-form';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { BlockButton, Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader } from '@/components/ui/card';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { cn } from '@/lib/utils';
import { zodResolver } from '@hookform/resolvers/zod';
import { ChevronDown, ChevronUp, Trash2 } from 'lucide-react';
import { memo, useCallback, useState } from 'react';
import {
  useFieldArray,
  useForm,
  useFormContext,
  useWatch,
} from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  initialTitleChunkerValues,
  TitleChunkerMethod,
} from '../../constant/pipeline';
import { useFormChangeCallback } from '../../hooks/use-form-change-callback';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { transformApiResponseToForm, useDynamicHierarchyOptions } from './hook';

const outputList = buildOutputList(initialTitleChunkerValues.outputs);

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
  method: z.nativeEnum(TitleChunkerMethod),
  hierarchyHierarchy: z.string().optional(),
  hierarchyGroup: z.string().optional(),
  include_heading_content: z.boolean().optional(),
  root_chunk_as_heading: z.boolean().optional(),
  hierarchyRules: rulesSchema,
  groupRules: rulesSchema,
});

export enum TitleChunkerRulesField {
  Hierarchy = 'hierarchyRules',
  Group = 'groupRules',
}

export type TitleChunkerFormSchemaType = z.infer<typeof FormSchema> & {
  rules: z.infer<typeof rulesSchema>;
  hierarchy: string;
};

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

  const handleRemove = useCallback(() => {
    removeParent(index);
  }, [removeParent, index]);

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
          onClick={handleRemove}
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

  const handleAppendLevel = useCallback(() => {
    appendLevel({ expression: '' });
  }, [appendLevel]);

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

      <BlockButton type="button" onClick={handleAppendLevel} className="mt-4">
        {t('flow.addRegularExpressions')}
      </BlockButton>
    </CardContent>
  );
}

type RulesFieldArrayProps = {
  name: TitleChunkerRulesField;
};

function RulesFieldArray({ name }: RulesFieldArrayProps) {
  const { t } = useTranslation();
  const form = useFormContext();
  const { fields, append, remove } = useFieldArray({
    name,
    control: form.control,
  });

  const handleAppendRule = useCallback(() => {
    append({
      levels: [{ expression: '' }],
    });
  }, [append]);

  return (
    <div className="space-y-4">
      {fields.map((cardField, cardIndex) => {
        const handleRemoveCard = () => remove(cardIndex);

        return (
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
                  onClick={handleRemoveCard}
                  className="h-7 w-7 p-0 text-muted-foreground hover:text-destructive"
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              )}
            </CardHeader>
            <CardBody cardIndex={cardIndex} cardName={`${name}.${cardIndex}`} />
          </Card>
        );
      })}
      <BlockButton type="button" onClick={handleAppendRule} className="mt-4">
        {t('flow.addRule', 'Add Rule')}
      </BlockButton>
    </div>
  );
}

const TitleChunkerForm = ({
  node,
  onValuesChange,
  hideOutputs,
}: INextOperatorForm) => {
  const { t } = useTranslation();
  const initialValues = useFormValues(initialTitleChunkerValues, node);

  const form = useForm<TitleChunkerFormSchemaType>({
    defaultValues: transformApiResponseToForm(initialValues),
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
  });
  const [showAllTip, setShowAllTip] = useState(true);

  const method = useWatch({ name: 'method', control: form.control });

  const activeRulesName =
    method === TitleChunkerMethod.Group
      ? TitleChunkerRulesField.Group
      : TitleChunkerRulesField.Hierarchy;

  const hierarchyOptions = useDynamicHierarchyOptions(form, activeRulesName);

  useWatchFormChange(node?.id, form);
  useFormChangeCallback(form, onValuesChange);

  const handleToggleShowAllTip = useCallback(() => {
    setShowAllTip((prev) => !prev);
  }, []);

  return (
    <Form {...form}>
      <FormWrapper>
        <RenderField
          field={{
            name: 'method',
            type: FormFieldType.Segmented,
            label: '',
            options: [
              {
                label: t('flow.hierarchy'),
                value: TitleChunkerMethod.Hierarchy,
              },
              // { label: t('flow.tree', 'Tree'), value: 'tree' },
              {
                label: t('flow.group', 'Group'),
                value: TitleChunkerMethod.Group,
              },
            ],
          }}
        />
        <div
          className={`text-xs text-text-secondary w-full cursor-pointer `}
          onClick={handleToggleShowAllTip}
        >
          <div className={cn('flex justify-start items-start')}>
            <div
              className={cn(
                'flex-1 ',
                showAllTip ? 'whitespace-pre-wrap' : 'truncate',
              )}
            >
              {method === TitleChunkerMethod.Hierarchy
                ? t('flow.hierarchyTip')
                : method === TitleChunkerMethod.Group
                  ? t('flow.groupTip')
                  : ''}
            </div>
            <div className="flex ml-2 text-xs ">
              {showAllTip ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
            </div>
          </div>
        </div>
        <RAGFlowFormItem
          name={'hierarchyHierarchy'}
          label={''}
          className={cn({ hidden: method !== TitleChunkerMethod.Hierarchy })}
        >
          <SelectWithSearch options={hierarchyOptions}></SelectWithSearch>
        </RAGFlowFormItem>

        <RAGFlowFormItem
          name={'hierarchyGroup'}
          label={''}
          className={cn({ hidden: method !== TitleChunkerMethod.Group })}
        >
          <SelectWithSearch options={hierarchyOptions}></SelectWithSearch>
        </RAGFlowFormItem>

        {method === TitleChunkerMethod.Hierarchy && (
          <>
            <RAGFlowFormItem
              name="include_heading_content"
              label={t('flow.includeHeadingContent', 'Include heading content')}
              tooltip={t('flow.includeHeadingContentTip')}
              horizontal={true}
              labelClassName="w-full"
              valueClassName="w-8"
            >
              {(field) => (
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            </RAGFlowFormItem>

            <RAGFlowFormItem
              name="root_chunk_as_heading"
              label={t('flow.rootAsHeading', 'Use root as heading')}
              tooltip={t(
                'flow.rootAsHeadingTip',
                'Treat the root node as a H0 heading when building the hierarchy',
              )}
              horizontal={true}
              labelClassName="w-full"
              valueClassName="w-8"
            >
              {(field) => (
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            </RAGFlowFormItem>
          </>
        )}
        <div
          className={
            method === TitleChunkerMethod.Hierarchy ? 'block' : 'hidden'
          }
        >
          <RulesFieldArray name={TitleChunkerRulesField.Hierarchy} />
        </div>
        <div
          className={method === TitleChunkerMethod.Group ? 'block' : 'hidden'}
        >
          <RulesFieldArray name={TitleChunkerRulesField.Group} />
        </div>
        {/* )} */}
      </FormWrapper>
      {!hideOutputs && (
        <div className="p-5">
          <Output list={outputList}></Output>
        </div>
      )}
    </Form>
  );
};

export default memo(TitleChunkerForm);
