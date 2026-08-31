import { AgentCategory } from '@/constants/agent';
import { LucideIcon, Network, Route, Shapes } from 'lucide-react';

export enum FlowType {
  Agent = 'agent',
  Compiler = 'compiler',
  Flow = 'flow',
}

export const FlowTypeConfig: Record<
  FlowType,
  { icon: LucideIcon; labelKey: string; color: string }
> = {
  [FlowType.Agent]: {
    icon: Network,
    labelKey: 'tabList.workflow',
    color: 'var(--team-member)',
  },
  [FlowType.Compiler]: {
    icon: Shapes,
    labelKey: 'tabList.compilationOperator',
    color: 'var(--team-department)',
  },
  [FlowType.Flow]: {
    icon: Route,
    labelKey: 'tabList.ingestionPipeline',
    color: 'var(--team-group)',
  },
};

/**
 * Map canvas_category → FlowType for resolving icons in agent cards.
 */
export const CanvasCategoryToFlowType: Record<string, FlowType> = {
  [AgentCategory.AgentCanvas]: FlowType.Agent,
  [AgentCategory.DataflowCanvas]: FlowType.Flow,
};
