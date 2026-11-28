import { PageFrame } from '@/components/page-frame';
import { Separator } from '@/components/ui/separator';
import { cn } from '@/lib/utils';
import { PropsWithChildren, ReactNode } from 'react';

type SidebarPageFrameProps = PropsWithChildren<{
  sidebar?: ReactNode;
  closeButton?: ReactNode;
  containerClassName?: string;
  panelClassName?: string;
  contentPanelClassName?: string;
}>;

export function SidebarPageFrame({
  sidebar,
  closeButton,
  containerClassName,
  panelClassName,
  contentPanelClassName,
  children,
}: SidebarPageFrameProps) {
  return (
    <div className="size-full p-2.5 [&>.page-frame]:pr-0 [&>.page-frame]:pb-0">
      <PageFrame className="relative">
        {closeButton}
        <div
          className={cn(
            'mx-auto min-h-0 min-w-0 w-full max-w-5xl flex-1',
            sidebar
              ? 'grid grid-cols-[4rem_1px_minmax(0,1fr)] gap-x-[clamp(0.5rem,2.5vw,2.5rem)] md:grid-cols-[16rem_1px_minmax(0,1fr)]'
              : 'flex',
            containerClassName,
          )}
        >
          {sidebar && (
            <div
              className={cn(
                'flex min-h-0 min-w-0 flex-col pb-6 pl-4 pt-16 md:pl-8',
                panelClassName,
              )}
            >
              {sidebar}
            </div>
          )}
          {sidebar && (
            <Separator
              orientation="vertical"
              className="w-[0.5px] bg-border-button"
            />
          )}
          <div
            className={cn(
              'flex min-h-0 min-w-0 flex-1 overflow-hidden pb-6 pr-4 pt-16 md:pr-8',
              panelClassName,
              contentPanelClassName,
            )}
          >
            {children}
          </div>
        </div>
      </PageFrame>
    </div>
  );
}
