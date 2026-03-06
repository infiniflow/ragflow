import { LucideCircleQuestionMark } from 'lucide-react';

import { Tooltip, TooltipContent, TooltipTrigger } from './ui/tooltip';

export default function WhatIsThis({ children }: React.PropsWithChildren<{}>) {
  return (
    <Tooltip>
      <TooltipTrigger>
        <LucideCircleQuestionMark className="size-[1em]" />
      </TooltipTrigger>

      <TooltipContent>{children}</TooltipContent>
    </Tooltip>
  );
}
