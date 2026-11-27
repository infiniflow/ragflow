import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { PropsWithChildren } from 'react';
import { useTranslation } from 'react-i18next';

export interface RunTooltipProps extends PropsWithChildren {
  tooltip?: string;
}

export const RunTooltip = ({
  children,
  tooltip = 'flow.testRun',
}: RunTooltipProps) => {
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
