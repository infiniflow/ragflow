import { BlockButton } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { PencilLine, X } from 'lucide-react';
import { PropsWithChildren } from 'react';
import { ToolPopover } from './tool-popover';
import { useDeleteAgentNodeTools } from './tool-popover/use-update-tools';
import { useGetAgentToolNames } from './use-get-tools';

export function ToolCard({
  children,
  className,
  ...props
}: PropsWithChildren & React.HTMLAttributes<HTMLLIElement>) {
  return (
    <li
      {...props}
      className={cn(
        'flex bg-background-card p-1 rounded-sm justify-between',
        className,
      )}
    >
      {children}
    </li>
  );
}

export function AgentTools() {
  const { toolNames } = useGetAgentToolNames();
  const { deleteNodeTool } = useDeleteAgentNodeTools();

  return (
    <section className="space-y-2.5">
      <span className="text-text-sub-title">Tools</span>
      <ul className="space-y-2">
        {toolNames.map((x) => (
          <ToolCard key={x}>
            {x}
            <div className="flex items-center gap-2 text-text-sub-title">
              <PencilLine className="size-4 cursor-pointer" />
              <X
                className="size-4 cursor-pointer"
                onClick={deleteNodeTool(x)}
              />
            </div>
          </ToolCard>
        ))}
      </ul>
      <ToolPopover>
        <BlockButton>Add Tool</BlockButton>
      </ToolPopover>
    </section>
  );
}
