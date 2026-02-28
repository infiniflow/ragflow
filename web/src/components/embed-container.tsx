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

  return (
    <section className="h-[100vh] flex justify-center items-center">
      <div className="hidden xl:flex w-40 gap-2 absolute left-3 top-12 items-center">
        <img src="/logo.svg" alt="" />
        <span className="text-2xl font-bold">{appConf.appName}</span>
      </div>
      <div className="w-full h-full md:w-[80vw] md:h-auto border-0 md:border rounded-none md:rounded-lg">
        <div className="flex justify-between items-center border-b p-3 relative">
          <div className="flex gap-2 items-center absolute left-1/2 -translate-x-1/2 md:static md:left-auto md:translate-x-0">
            <RAGFlowAvatar
              avatar={avatar}
              name={title}
              isPerson
              className="size-5 md:size-10"
            />
            <div className="md:text-xl text-foreground">{title}</div>
          </div>
          <div className="flex md:hidden items-center">
            <img src="/logo.svg" alt="" className="h-6" />
          </div>
          <Button
            variant={'secondary'}
            className="text-sm text-foreground cursor-pointer"
            onClick={handleReset}
          >
            <div className="flex gap-1 items-center">
              <RefreshCcw size={14} />
              <span className="hidden text-lg md:inline-block">Reset</span>
            </div>
          </Button>
        </div>
        {children}
      </div>
    </section>
  );
}
