import { CardContainer } from '@/components/card-container';
import { Collapse } from '@/components/collapse';
import { useFetchWikiPresets } from '@/hooks/use-compilation-template-request';
import { ICompilationTemplateBuiltin } from '@/interfaces/database/compilation-template';
import { UseFormReturn } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { useBlueprintSelection } from '../hooks/use-blueprint-selection';
import { FormSchemaType } from '../schema';
import { BlueprintCard } from './blueprint-card';
import { BlueprintDialog } from './blueprint-dialog';

type BlueprintSectionProps = {
  form: UseFormReturn<FormSchemaType>;
  builtins: ICompilationTemplateBuiltin[];
  selectedTemplateIndex: number;
};

export function BlueprintSection({
  form,
  builtins,
  selectedTemplateIndex,
}: BlueprintSectionProps) {
  const { t } = useTranslation();
  const { data: presets } = useFetchWikiPresets();
  const {
    selectedPresetId,
    previewPreset,
    handleTogglePreset,
    handlePreview,
    handlePreviewOpenChange,
  } = useBlueprintSelection({ form, builtins, selectedTemplateIndex });

  if (presets.length === 0) {
    return null;
  }

  return (
    <section className="space-y-4 pt-4">
      <Collapse
        defaultOpen
        title={
          <h3 className="text-base font-medium">{t('setting.blueprints')}</h3>
        }
      >
        <CardContainer>
          {presets.map((preset) => (
            <BlueprintCard
              key={preset.id}
              preset={preset}
              selected={preset.id === selectedPresetId}
              onToggle={handleTogglePreset}
              onPreview={handlePreview}
            />
          ))}
        </CardContainer>
      </Collapse>

      <BlueprintDialog
        preset={previewPreset}
        onOpenChange={handlePreviewOpenChange}
      />
    </section>
  );
}
