import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { CompilationTemplateFormValues } from '../interface';

/**
 * RAPTOR-style knobs (summarization prompt + max_token + threshold).
 * Only mounted when `kind === 'tree'`. Mirrors the artifact-extras
 * pattern so the conditional rendering tree stays clean for other
 * kinds.
 */
export function TreeExtras() {
  const form = useFormContext<CompilationTemplateFormValues>();
  const { t } = useTranslation();

  return (
    <section className="space-y-3">
      <h3 className="text-sm font-medium">
        {t('knowledgeCompilation.treeSectionTitle')}
      </h3>

      <FormField
        control={form.control}
        name="raptor.prompt"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('knowledgeCompilation.treePromptLabel')}</FormLabel>
            <FormControl>
              <Textarea
                {...field}
                rows={6}
                placeholder={t('knowledgeCompilation.treePromptPlaceholder')}
                className="font-mono text-sm"
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <div className="grid grid-cols-2 gap-3">
        <FormField
          control={form.control}
          name="raptor.max_token"
          render={({ field }) => (
            <FormItem>
              <FormLabel>
                {t('knowledgeCompilation.treeMaxTokenLabel')}
              </FormLabel>
              <FormControl>
                <Input
                  type="number"
                  min={1}
                  max={8192}
                  step={1}
                  {...field}
                  value={field.value ?? 512}
                  onChange={(e) => field.onChange(Number(e.target.value))}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name="raptor.threshold"
          render={({ field }) => (
            <FormItem>
              <FormLabel>
                {t('knowledgeCompilation.treeThresholdLabel')}
              </FormLabel>
              <FormControl>
                <Input
                  type="number"
                  min={0}
                  max={1}
                  step={0.01}
                  {...field}
                  value={field.value ?? 0.1}
                  onChange={(e) => field.onChange(Number(e.target.value))}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>
    </section>
  );
}
