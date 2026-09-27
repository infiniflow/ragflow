import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { PropsWithChildren } from 'react';
import { useTranslation } from 'react-i18next';

export interface RunTooltipProps extends PropsWithChildren {
  /**
   * i18n key for the tooltip copy. Defaults to "flow.testRun" (the existing
   * test-run tooltip). The canvas "Run" button passes "flow.debugRunLimits"
   * to describe the Go-side debug (dry-run) preview limits.
   */
  tooltip?: string;
}

export const RunTooltip = ({ children, tooltip = 'flow.testRun' }: RunTooltipProps) => {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent>
        <p>{t(tooltip)}</p>
      </TooltipContent>
    </Tooltip>
  );
};
