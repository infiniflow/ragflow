import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { CompilationTemplateFormValues, TEXT_FIELD_MAX } from '../interface';
import { FieldListBlock } from './field-list-block';

/**
 * Claim + Concept blocks. Only mounted when `kind === 'artifacts'`.
 * Render-side conditional in the parent form keeps the React tree clean
 * when other kinds are selected.
 */
export function ArtifactExtras() {
  const form = useFormContext<CompilationTemplateFormValues>();
  const { t } = useTranslation();

  const claimArray = useFieldArray({
    control: form.control,
    name: 'claim.fields',
  });
  const conceptArray = useFieldArray({
    control: form.control,
    name: 'concept.fields',
  });

  return (
    <>
      {/* "Page-structure example" lives in the parent form, right
          under the LLM picker — see edit-template-form.tsx. Kept out
          of ArtifactExtras so the example sits visually above the
          Entity/Relation sections. */}
      <section className="flex flex-col gap-4 rounded-md border border-border-button p-4">
        <h3 className="text-base font-medium">
          {t('knowledgeCompilation.claimSpecification')}
        </h3>
        <FieldListBlock
          items={claimArray.fields}
          onAdd={() => claimArray.append({ statement: '', subject: '' })}
          onRemove={(index) => claimArray.remove(index)}
          addLabel={t('knowledgeCompilation.addField')}
          renderItem={(_item, index) => (
            <div className="flex flex-col gap-3">
              <FormField
                control={form.control}
                name={`claim.fields.${index}.statement` as const}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel className="text-xs">
                      {t('knowledgeCompilation.statement')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        maxLength={TEXT_FIELD_MAX}
                        placeholder={t(
                          'knowledgeCompilation.statementPlaceholder',
                        )}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name={`claim.fields.${index}.subject` as const}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel className="text-xs">
                      {t('knowledgeCompilation.subject')}
                    </FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        rows={2}
                        maxLength={TEXT_FIELD_MAX}
                        placeholder={t(
                          'knowledgeCompilation.subjectPlaceholder',
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

      <section className="flex flex-col gap-4 rounded-md border border-border-button p-4">
        <h3 className="text-base font-medium">
          {t('knowledgeCompilation.conceptSpecification')}
        </h3>
        <FieldListBlock
          items={conceptArray.fields}
          onAdd={() =>
            conceptArray.append({ term: '', definition_excerpt: '' })
          }
          onRemove={(index) => conceptArray.remove(index)}
          addLabel={t('knowledgeCompilation.addField')}
          renderItem={(_item, index) => (
            <div className="flex flex-col gap-3">
              <FormField
                control={form.control}
                name={`concept.fields.${index}.term` as const}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel className="text-xs">
                      {t('knowledgeCompilation.term')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        maxLength={TEXT_FIELD_MAX}
                        placeholder={t('knowledgeCompilation.termPlaceholder')}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name={`concept.fields.${index}.definition_excerpt` as const}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel className="text-xs">
                      {t('knowledgeCompilation.definitionExcerpt')}
                    </FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        rows={2}
                        maxLength={TEXT_FIELD_MAX}
                        placeholder={t(
                          'knowledgeCompilation.definitionExcerptPlaceholder',
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
    </>
  );
}
