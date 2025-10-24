import { useFetchAppConf } from '@/hooks/logic-hooks';
import { RefreshCcw } from 'lucide-react';
import { PropsWithChildren } from 'react';
import { RAGFlowAvatar } from './ragflow-avatar';
import { Button } from './ui/button';

type EmbedContainerProps = {
  title: string;
  avatar?: string;
  handleReset?(): void;
} & PropsWithChildren;

export function EmbedContainer({
  title,
  avatar,
  children,
  handleReset,
}: EmbedContainerProps) {
  const appConf = useFetchAppConf();
  const hideBranding = new URLSearchParams(location.search).get('hide_branding') === '1';

  return (
    <section className="h-[100vh] flex justify-center items-center">
      {!hideBranding && (
        <div className="w-40 flex gap-2 absolute left-3 top-12 items-center">
          <img src="/logo.svg" alt="" />
          <span className="text-2xl font-bold">{appConf.appName}</span>
        </div>
      )}
      <div className="w-full md:w-[80vw] border rounded-lg">
        <div className="flex justify-between items-center border-b p-2 md:p-3">
          <div className="flex gap-1 md:gap-2 items-center">
            <RAGFlowAvatar avatar={avatar} name={title} isPerson className="h-8 w-8 md:h-10 md:w-10" />
            <div className="text-base md:text-xl text-foreground">{title}</div>
          </div>
          <Button
            variant={'secondary'}
            className="text-xs md:text-sm text-foreground cursor-pointer"
            onClick={handleReset}
          >
            <div className="flex gap-1 items-center">
              <RefreshCcw size={12} className="md:w-[14px] md:h-[14px]" />
              <span className="text-sm md:text-lg">Reset</span>
            </div>
          </Button>
        </div>
        {children}
      </div>
    </section>
  );
}
