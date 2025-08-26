import { BlockButton } from '@/components/ui/button';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { Position } from '@xyflow/react';
import { t } from 'i18next';
import { PencilLine, X } from 'lucide-react';
import {
  MouseEventHandler,
  PropsWithChildren,
  useCallback,
  useContext,
  useMemo,
} from 'react';
import { Operator } from '../../constant';
import { AgentInstanceContext } from '../../context';
import { useFindMcpById } from '../../hooks/use-find-mcp-by-id';
import { INextOperatorForm } from '../../interface';
import OperatorIcon from '../../operator-icon';
import useGraphStore from '../../store';
import { filterDownstreamAgentNodeIds } from '../../utils/filter-downstream-nodes';
import { ToolPopover } from './tool-popover';
import { useDeleteAgentNodeMCP } from './tool-popover/use-update-mcp';
import { useDeleteAgentNodeTools } from './tool-popover/use-update-tools';
import { useGetAgentMCPIds, useGetAgentToolNames } from './use-get-tools';

export function ToolCard({
  children,
  className,
  ...props
}: PropsWithChildren & React.HTMLAttributes<HTMLLIElement>) {
  const element = useMemo(() => {
    return (
      <li
        {...props}
        className={cn(
          'flex bg-bg-card p-1 rounded-sm justify-between',
          className,
        )}
      >
        {children}
      </li>
    );
  }, [children, className, props]);

  if (children === Operator.Code) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>{element}</TooltipTrigger>
        <TooltipContent>
          <p>It doesn't have any config.</p>
        </TooltipContent>
      </Tooltip>
    );
  }

  return element;
}

type ActionButtonProps<T> = {
  record: T;
  deleteRecord(record: T): void;
  edit: MouseEventHandler<HTMLOrSVGElement>;
};

function ActionButton<T>({ deleteRecord, record, edit }: ActionButtonProps<T>) {
  const handleDelete = useCallback(() => {
    deleteRecord(record);
  }, [deleteRecord, record]);

  return (
    <div className="flex items-center gap-2 text-text-secondary">
      <PencilLine
        className="size-4 cursor-pointer"
        data-tool={record}
        onClick={edit}
      />
      <X className="size-4 cursor-pointer" onClick={handleDelete} />
    </div>
  );
}

export function AgentTools() {
  const { toolNames } = useGetAgentToolNames();
  const { deleteNodeTool } = useDeleteAgentNodeTools();
  const { mcpIds } = useGetAgentMCPIds();
  const { findMcpById } = useFindMcpById();
  const { deleteNodeMCP } = useDeleteAgentNodeMCP();
  const { showFormDrawer } = useContext(AgentInstanceContext);
  const { clickedNodeId, findAgentToolNodeById, selectNodeIds } = useGraphStore(
    (state) => state,
  );

  const handleEdit: MouseEventHandler<SVGSVGElement> = useCallback(
    (e) => {
      const toolNodeId = findAgentToolNodeById(clickedNodeId);
      if (toolNodeId) {
        selectNodeIds([toolNodeId]);
        showFormDrawer(e, toolNodeId);
      }
    },
    [clickedNodeId, findAgentToolNodeById, selectNodeIds, showFormDrawer],
  );

  return (
    <section className="space-y-2.5">
      <span className="text-text-secondary">{t('flow.tools')}</span>
      <ul className="space-y-2">
        {toolNames.map((x) => (
          <ToolCard key={x}>
            <div className="flex gap-2 items-center">
              <OperatorIcon name={x as Operator}></OperatorIcon>
              {x}
            </div>
            <ActionButton
              record={x}
              deleteRecord={deleteNodeTool(x)}
              edit={handleEdit}
            ></ActionButton>
          </ToolCard>
        ))}
        {mcpIds.map((id) => (
          <ToolCard key={id}>
            {findMcpById(id)?.name}
            <ActionButton
              record={id}
              deleteRecord={deleteNodeMCP(id)}
              edit={handleEdit}
            ></ActionButton>
          </ToolCard>
        ))}
      </ul>
      <ToolPopover>
        <BlockButton>{t('flow.addTools')}</BlockButton>
      </ToolPopover>
    </section>
  );
}

export function Agents({ node }: INextOperatorForm) {
  const { addCanvasNode } = useContext(AgentInstanceContext);
  const { deleteAgentDownstreamNodesById, edges, getNode, selectNodeIds } =
    useGraphStore((state) => state);
  const { showFormDrawer } = useContext(AgentInstanceContext);

  const handleEdit = useCallback(
    (nodeId: string): MouseEventHandler<SVGSVGElement> =>
      (e) => {
        selectNodeIds([nodeId]);
        showFormDrawer(e, nodeId);
      },
    [selectNodeIds, showFormDrawer],
  );

  const subBottomAgentNodeIds = useMemo(() => {
    return filterDownstreamAgentNodeIds(edges, node?.id);
  }, [edges, node?.id]);

  return (
    <section className="space-y-2.5">
      <span className="text-text-secondary">{t('flow.agent')}</span>
      <ul className="space-y-2">
        {subBottomAgentNodeIds.map((id) => {
          const currentNode = getNode(id);

          return (
            <ToolCard key={id}>
              {currentNode?.data.name}
              <ActionButton
                record={id}
                deleteRecord={deleteAgentDownstreamNodesById}
                edit={handleEdit(id)}
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
        {t('flow.addAgent')}
      </BlockButton>
    </section>
  );
}
