import { Button } from '@/components/ui/button';
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { Textarea } from '@/components/ui/textarea';
import { cn } from '@/lib/utils';
import { Check, ChevronsUpDown } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import {
  CompilationTemplateFormValues,
  ENTITY_TYPE_OPTIONS,
  FieldTemplateMap,
  RELATION_TYPE_OPTIONS,
  TEXT_FIELD_MAX,
  TYPE_FIELD_MAX,
} from '../interface';
import { FieldListBlock } from './field-list-block';

type SectionVariant = 'entity' | 'relation';

interface EntityRelationSectionProps {
  variant: SectionVariant;
  kind: CompilationTemplateFormValues['kind'];
  fieldTemplates: FieldTemplateMap;
}

interface TypeComboboxProps {
  value: string;
  options: readonly string[];
  placeholder: string;
  onChange: (value: string) => void;
}

function TypeCombobox({
  value,
  options,
  placeholder,
  onChange,
}: TypeComboboxProps) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState('');

  const visibleOptions = useMemo(() => {
    const normalized = search.trim().toLowerCase();
    if (!normalized) {
      return options;
    }
    return options.filter((option) =>
      option.toLowerCase().includes(normalized),
    );
  }, [options, search]);

  const handleValueChange = useCallback(
    (nextValue: string) => {
      onChange(nextValue);
      setOpen(false);
      setSearch('');
    },
    [onChange],
  );

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className="w-full justify-between font-normal"
        >
          <span className={cn('truncate', !value && 'text-text-secondary')}>
            {value || placeholder}
          </span>
          <ChevronsUpDown className="size-3.5 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[--radix-popover-trigger-width] p-0">
        <Command shouldFilter={false}>
          <CommandInput
            value={search}
            placeholder={placeholder}
            maxLength={TYPE_FIELD_MAX}
            onValueChange={(nextSearch) => {
              setSearch(nextSearch);
              onChange(nextSearch);
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && search.trim()) {
                e.preventDefault();
                handleValueChange(search.trim());
              }
            }}
          />
          <CommandList>
            <CommandEmpty>No results found.</CommandEmpty>
            <CommandGroup>
              {visibleOptions.map((option) => (
                <CommandItem
                  key={option}
                  value={option}
                  onSelect={() => handleValueChange(option)}
                >
                  <Check
                    className={cn(
                      'size-3.5',
                      value === option ? 'opacity-100' : 'opacity-0',
                    )}
                  />
                  {option}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

/**
 * One of the two top-level "specification" blocks. Entity and Relation
 * share the same `(description, fields[])` shape, so they're rendered by
 * the same component with a discriminator.
 */
export function EntityRelationSection({
  variant,
  fieldTemplates,
}: EntityRelationSectionProps) {
  const form = useFormContext<CompilationTemplateFormValues>();
  const { t } = useTranslation();

  const fieldArray = useFieldArray({
    control: form.control,
    name: `${variant}.fields` as const,
  });

  const typeOptions =
    variant === 'entity' ? ENTITY_TYPE_OPTIONS : RELATION_TYPE_OPTIONS;

  const applyTypeTemplate = useCallback(
    (index: number, nextType: string) => {
      const template = fieldTemplates[nextType];
      if (!template) {
        return;
      }
      form.setValue(
        `${variant}.fields.${index}.description`,
        template.description,
        { shouldDirty: true, shouldValidate: true },
      );
      form.setValue(`${variant}.fields.${index}.rule`, template.rule, {
        shouldDirty: true,
        shouldValidate: true,
      });
    },
    [fieldTemplates, form, variant],
  );

  return (
    <section className="flex flex-col gap-4 rounded-md border border-border-button p-4">
      <h3 className="text-base font-medium">
        {variant === 'entity'
          ? t('knowledgeCompilation.entitySpecification')
          : t('knowledgeCompilation.relationSpecification')}
      </h3>

      <FormField
        control={form.control}
        name={`${variant}.description` as const}
        render={({ field }) => (
          <FormItem>
            <FormLabel>
              {t('knowledgeCompilation.sectionDescription')}
            </FormLabel>
            <FormControl>
              <Textarea
                {...field}
                maxLength={TEXT_FIELD_MAX}
                rows={3}
                placeholder={t(
                  variant === 'entity'
                    ? 'knowledgeCompilation.entityDescriptionPlaceholder'
                    : 'knowledgeCompilation.relationDescriptionPlaceholder',
                )}
              />
            </FormControl>
            <p className="text-xs text-text-secondary text-right">
              {(field.value ?? '').length}/{TEXT_FIELD_MAX}
            </p>
            <FormMessage />
          </FormItem>
        )}
      />

      <FieldListBlock
        items={fieldArray.fields}
        onAdd={() => fieldArray.append({ type: '', description: '', rule: '' })}
        onRemove={(index) => fieldArray.remove(index)}
        addLabel={t('knowledgeCompilation.addField')}
        minItems={1}
        renderItem={(_item, index) => (
          <div className="flex flex-col gap-3">
            <FormField
              control={form.control}
              name={`${variant}.fields.${index}.type` as const}
              render={({ field }) => (
                <FormItem>
                  <FormLabel className="text-xs">
                    {t('knowledgeCompilation.fieldType')}
                  </FormLabel>
                  <FormControl>
                    <TypeCombobox
                      value={field.value ?? ''}
                      options={typeOptions}
                      placeholder={t(
                        'knowledgeCompilation.fieldTypePlaceholder',
                      )}
                      onChange={(nextValue) => {
                        field.onChange(nextValue);
                        applyTypeTemplate(index, nextValue);
                      }}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name={`${variant}.fields.${index}.description` as const}
              render={({ field }) => (
                <FormItem>
                  <FormLabel className="text-xs">
                    {t('knowledgeCompilation.fieldDescription')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      maxLength={TEXT_FIELD_MAX}
                      placeholder={t(
                        'knowledgeCompilation.fieldDescriptionPlaceholder',
                      )}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name={`${variant}.fields.${index}.rule` as const}
              render={({ field }) => (
                <FormItem>
                  <FormLabel className="text-xs">
                    {t('knowledgeCompilation.fieldRule')}
                  </FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      rows={3}
                      maxLength={TEXT_FIELD_MAX}
                      placeholder={t(
                        'knowledgeCompilation.fieldRulePlaceholder',
                      )}
                    />
                  </FormControl>
                  <p className="text-xs text-text-secondary text-right">
                    {(field.value ?? '').length}/{TEXT_FIELD_MAX}
                  </p>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
        )}
      />
    </section>
  );
}
