import { useFetchAppConf } from '@/hooks/logic-hooks';
import { RefreshCcw } from 'lucide-react';
import { PropsWithChildren } from 'react';
import { RAGFlowAvatar } from './ragflow-avatar';
import { Button } from './ui/button';

type EmbedContainerProps = {
  title: string;
  avatar?: string;
  description?: string;
  handleReset?(): void;
} & PropsWithChildren;

export function EmbedContainer({
  title,
  avatar,
  description,
  children,
  handleReset,
}: EmbedContainerProps) {
  const appConf = useFetchAppConf();

  return (
    <section className="h-[100vh] flex justify-center items-center">
      {/* Temporarily hidden for portal embed view */}
      {/* <div className="flex gap-2 absolute left-3 top-12 items-center">
        <img src="/logo.gif" alt="" className="w-8 h-8 object-contain" />
        <span className="text-2xl font-bold">{appConf.appName}</span>
      </div> */}
      <div className=" w-full border rounded-lg">
        <div className="flex justify-between items-center border-b p-3">
          <div className="flex gap-2 items-center flex-1 min-w-0">
            <RAGFlowAvatar avatar={avatar} name={title} isPerson />
            <div className="flex-1 min-w-0">
              <div className="text-xl text-foreground">{title}</div>
              {description && (
                <div className="text-sm text-muted-foreground mt-1">
                  {description}
                </div>
              )}
            </div>
          </div>
          <Button
            variant={'secondary'}
            className="text-sm text-foreground cursor-pointer flex-shrink-0 ml-2"
            onClick={handleReset}
          >
            <div className="flex gap-1 items-center">
              <RefreshCcw size={14} />
              <span className="text-lg ">重新对话</span>
            </div>
          </Button>
        </div>
        {children}
      </div>
    </section>
  );
}
