import { useTranslation } from 'react-i18next';

import { JsonPreviewSheet } from './json-preview-sheet';
import { WorkflowPreviewSheet } from './workflow-preview-sheet';

interface TemplatePreviewHeaderProps {
  templateName: string | undefined;
  jsonSheetOpen: boolean;
  onJsonSheetOpenChange: (open: boolean) => void;
  workflowSheetOpen: boolean;
  onWorkflowSheetOpenChange: (open: boolean) => void;
  allFormValues: Record<string, unknown>;
}

export function TemplatePreviewHeader({
  templateName,
  jsonSheetOpen,
  onJsonSheetOpenChange,
  workflowSheetOpen,
  onWorkflowSheetOpenChange,
  allFormValues,
}: TemplatePreviewHeaderProps) {
  const { t } = useTranslation();

  return (
    <section className="shrink-0 flex justify-between items-center px-5 py-4 border-b border-border-button">
      <span className="text-lg font-medium text-text-primary">
        {templateName || t('setting.templateName')}
      </span>
      <div className="flex items-center gap-2">
        <JsonPreviewSheet
          open={jsonSheetOpen}
          onOpenChange={onJsonSheetOpenChange}
          value={allFormValues}
        />
        <WorkflowPreviewSheet
          open={workflowSheetOpen}
          onOpenChange={onWorkflowSheetOpenChange}
        />
      </div>
    </section>
  );
}
