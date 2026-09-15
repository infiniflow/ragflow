import {
  type IStructureGraphTemplate,
  type StructureTemplateKind,
} from '@/interfaces/database/document-structure';
import { useMemo, useState } from 'react';

export function useSelectedTemplate(templates: IStructureGraphTemplate[]) {
  const [selectedTemplateId, setSelectedTemplateId] = useState<string>('');

  const effectiveSelectedTemplateId = useMemo(() => {
    // Keywords search returns a single bucket that may not be the selected
    // template — fall back to the first template instead of rendering nothing.
    if (
      templates.some((template) => template.template_id === selectedTemplateId)
    ) {
      return selectedTemplateId;
    }
    return templates[0]?.template_id || '';
  }, [selectedTemplateId, templates]);

  const selectedTemplate = useMemo(() => {
    return templates.find(
      (template) => template.template_id === effectiveSelectedTemplateId,
    );
  }, [templates, effectiveSelectedTemplateId]);

  const selectedKind = useMemo(
    () => selectedTemplate?.kind ?? ('' as StructureTemplateKind),
    [selectedTemplate],
  );

  return {
    selectedTemplateId: effectiveSelectedTemplateId,
    setSelectedTemplateId,
    selectedTemplate,
    selectedKind,
  };
}
