import { BlockButton } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { Position } from '@xyflow/react';
import { PencilLine, X } from 'lucide-react';
import { PropsWithChildren, useCallback, useContext, useMemo } from 'react';
import { Operator } from '../../constant';
import { AgentInstanceContext } from '../../context';
import { INextOperatorForm } from '../../interface';
import useGraphStore from '../../store';
import { filterDownstreamAgentNodeIds } from '../../utils/filter-downstream-nodes';
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

type ActionButtonProps<T> = {
  record: T;
  deleteRecord(record: T): void;
  edit(record: T): void;
};

function ActionButton<T>({ edit, deleteRecord, record }: ActionButtonProps<T>) {
  const handleDelete = useCallback(() => {
    deleteRecord(record);
  }, [deleteRecord, record]);
  const handleEdit = useCallback(() => {
    edit(record);
  }, [edit, record]);

  return (
    <div className="flex items-center gap-2 text-text-sub-title">
      <PencilLine
        className="size-4 cursor-pointer"
        data-tool={record}
        onClick={handleEdit}
      />
      <X className="size-4 cursor-pointer" onClick={handleDelete} />
    </div>
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
            <ActionButton
              record={x}
              edit={() => {}}
              deleteRecord={deleteNodeTool(x)}
            ></ActionButton>
          </ToolCard>
        ))}
      </ul>
      <ToolPopover>
        <BlockButton>Add Tool</BlockButton>
      </ToolPopover>
    </section>
  );
}

export function Agents({ node }: INextOperatorForm) {
  const { addCanvasNode } = useContext(AgentInstanceContext);
  const { deleteAgentDownstreamNodesById, edges, getNode } = useGraphStore(
    (state) => state,
  );

  const subBottomAgentNodeIds = useMemo(() => {
    return filterDownstreamAgentNodeIds(edges, node?.id);
  }, [edges, node?.id]);

  return (
    <section className="space-y-2.5">
      <span className="text-text-sub-title">Agents</span>
      <ul className="space-y-2">
        {subBottomAgentNodeIds.map((id) => {
          const currentNode = getNode(id);

          return (
            <ToolCard key={id}>
              {currentNode?.data.name}
              <ActionButton
                record={id}
                edit={() => {}}
                deleteRecord={deleteAgentDownstreamNodesById}
              ></ActionButton>
            </ToolCard>
          );
        })}
      </ul>
      <BlockButton
        onClick={addCanvasNode(Operator.Agent, {
          nodeId: node?.id,
          position: Position.Bottom,
        })}
      >
        Add Agent
      </BlockButton>
    </section>
  );
}
