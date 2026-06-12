import {
  BuiltinCompilationTemplate,
  CompilationTemplateConfig,
} from '@/interfaces/database/compilation-template';
import { useCallback, useState } from 'react';
import { UseFormReturn } from 'react-hook-form';
import {
  CompilationTemplateFormValues,
  templateConfigToFormValues,
} from '../interface';

/**
 * Owns the "apply built-in template" workflow with a data-loss confirmation
 * gate. Apply order:
 *
 *   1. User picks a built-in entry from the popover.
 *   2. If the form is currently dirty (RHF's `formState.isDirty === true`)
 *      we stash the chosen template in `pendingBuiltin` and surface a
 *      confirmation dialog.
 *   3. If the user confirms (or the form was clean to begin with), we
 *      `reset()` the form with the built-in's config — RHF then treats
 *      the new state as the pristine baseline, so dirty tracking restarts.
 */
export function useTemplateFormState(
  form: UseFormReturn<CompilationTemplateFormValues>,
  initialName: string,
  initialDescription: string,
) {
  const [pendingBuiltin, setPendingBuiltin] =
    useState<BuiltinCompilationTemplate | null>(null);

  const applyBuiltin = useCallback(
    (template: BuiltinCompilationTemplate) => {
      const next = templateConfigToFormValues(
        template.display_name || form.getValues('name') || initialName,
        template.description ??
          form.getValues('description') ??
          initialDescription,
        template.config,
      );
      form.reset(next, { keepDefaultValues: false });
    },
    [form, initialName, initialDescription],
  );

  const handleSelectBuiltin = useCallback(
    (template: BuiltinCompilationTemplate) => {
      if (form.formState.isDirty) {
        setPendingBuiltin(template);
      } else {
        applyBuiltin(template);
      }
    },
    [form.formState.isDirty, applyBuiltin],
  );

  const confirmApplyPendingBuiltin = useCallback(() => {
    if (pendingBuiltin) {
      applyBuiltin(pendingBuiltin);
      setPendingBuiltin(null);
    }
  }, [pendingBuiltin, applyBuiltin]);

  const cancelPendingBuiltin = useCallback(() => {
    setPendingBuiltin(null);
  }, []);

  /**
   * Convenience for direct (non-dirty-checked) seeding — used when the
   * editor first opens with no existing template to load.
   */
  const seedFromConfig = useCallback(
    (
      config: CompilationTemplateConfig,
      name: string = initialName,
      description: string = initialDescription,
    ) => {
      form.reset(templateConfigToFormValues(name, description, config));
    },
    [form, initialName, initialDescription],
  );

  return {
    pendingBuiltin,
    handleSelectBuiltin,
    confirmApplyPendingBuiltin,
    cancelPendingBuiltin,
    seedFromConfig,
  };
}
