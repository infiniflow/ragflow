import { AgentCategory } from '@/constants/agent';
import { LucideIcon, Network, Route, Shapes } from 'lucide-react';

export enum FlowType {
  Agent = 'agent',
  Compiler = 'compiler',
  Flow = 'flow',
}

export const FlowTypeConfig: Record<
  FlowType,
  { icon: LucideIcon; labelKey: string }
> = {
  [FlowType.Agent]: { icon: Network, labelKey: 'tabList.workflow' },
  [FlowType.Compiler]: {
    icon: Shapes,
    labelKey: 'tabList.compilationOperator',
  },
  [FlowType.Flow]: { icon: Route, labelKey: 'tabList.ingestionPipeline' },
};

/**
 * Map canvas_category → FlowType for resolving icons in agent cards.
 */
export const CanvasCategoryToFlowType: Record<string, FlowType> = {
  [AgentCategory.AgentCanvas]: FlowType.Agent,
  [AgentCategory.DataflowCanvas]: FlowType.Flow,
};
