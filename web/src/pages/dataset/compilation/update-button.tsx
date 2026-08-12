import { WandSparkles } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { GenerateStatus, GenerateType } from '@/constants/knowledge';
import { ITraceInfo, useGenerateStatus } from '@/hooks/use-dataset-generate';
import { isGoDatasetBackend } from '@/utils/api-proxy-scheme';

import { UpdateRunProgress } from './update-run-progress';

type CompilationUpdateButtonProps = {
  traceData?: ITraceInfo;
  generateType: GenerateType;
  hasChanges: boolean;
  newlyUploaded: number;
  removed: number;
  loading: boolean;
  tooltip: string;
  onClick: () => void;
};

export function CompilationUpdateButton({
  traceData,
  generateType,
  hasChanges,
  newlyUploaded,
  removed,
  loading,
  tooltip,
  onClick,
}: CompilationUpdateButtonProps) {
  const { t } = useTranslation();
  const { status } = useGenerateStatus(traceData);
  const isRunning = status === GenerateStatus.Running;
  const isGenerating = isRunning || status === GenerateStatus.Failed;

  // Go/hybrid: compilation is auto-driven by the scheduler with no manual
  // re-merge, so hide the update control rather than offer a trigger that
  // cannot work (plan v4.1 §4.2).
  if (isGoDatasetBackend()) {
    return null;
  }

  // A failed trace persists (progress stays < 0) until the next run, so it
  // must not keep the button visible on its own — only real changes or a
  // live run should.
  if (!hasChanges && !isRunning) {
    return null;
  }

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant={'outline'} onClick={onClick} disabled={loading}>
            {isGenerating ? (
              <UpdateRunProgress data={traceData} generateType={generateType} />
            ) : (
              <>
                {t('knowledgeDetails.update', { defaultValue: 'Update' })}
                {newlyUploaded > 0 && (
                  <Badge variant="success" className="ml-1">
                    {newlyUploaded}
                  </Badge>
                )}
                {removed > 0 && (
                  <Badge variant="destructive" className="ml-1">
                    {removed}
                  </Badge>
                )}
                <WandSparkles />
              </>
            )}
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          {isGenerating
            ? t('knowledgeDetails.viewUpdateLogs', {
                defaultValue: 'View update logs',
              })
            : tooltip}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
