import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Textarea } from '@/components/ui/textarea';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import {
  CompilationTemplateFormValues,
  GLOBAL_RULES_FIELD_MAX,
} from '../interface';

/**
 * Global compilation rules — a single capped textarea.
 * Distinguished from per-field rules: this section applies to the whole
 * extraction, not to one entity/relation type.
 */
export function GlobalRulesBlock() {
  const form = useFormContext<CompilationTemplateFormValues>();
  const { t } = useTranslation();

  return (
    <FormField
      control={form.control}
      name="global_rules"
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t('knowledgeCompilation.globalRules')}</FormLabel>
          <FormControl>
            <Textarea
              {...field}
              maxLength={GLOBAL_RULES_FIELD_MAX}
              rows={4}
              placeholder={t('knowledgeCompilation.globalRulesPlaceholder')}
            />
          </FormControl>
          <p className="text-xs text-text-secondary text-right">
            {(field.value ?? '').length}/{GLOBAL_RULES_FIELD_MAX}
          </p>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
