import { AgentCategory } from '@/constants/agent';
import { AgentListItemType } from '@/interfaces/database/agent';
import { LucideIcon, Network, Route, Shapes } from 'lucide-react';

export enum FlowType {
  Agent = 'agent',
  Compiler = 'compiler',
  Flow = 'flow',
}

export type AgentTabCategory =
  | AgentCategory
  | AgentListItemType.CompilationTemplateGroup;

export const FlowTypeConfig: Record<
  FlowType,
  {
    icon: LucideIcon;
    labelKey: string;
    createTitleKey: string;
    color: string;
    canvasCategory: AgentTabCategory;
    supportsTemplates: boolean;
    supportsImport: boolean;
  }
> = {
  [FlowType.Agent]: {
    icon: Network,
    labelKey: 'tabList.workflow',
    createTitleKey: 'createWorkflow',
    color: 'var(--team-member)',
    canvasCategory: AgentCategory.AgentCanvas,
    supportsTemplates: true,
    supportsImport: true,
  },
  [FlowType.Compiler]: {
    icon: Shapes,
    labelKey: 'tabList.compilationOperator',
    createTitleKey: 'createCompilationOperator',
    color: 'var(--team-department)',
    canvasCategory: AgentListItemType.CompilationTemplateGroup,
    supportsTemplates: false,
    supportsImport: false,
  },
  [FlowType.Flow]: {
    icon: Route,
    labelKey: 'tabList.ingestionPipeline',
    createTitleKey: 'createIngestionPipeline',
    color: 'var(--team-group)',
    canvasCategory: AgentCategory.DataflowCanvas,
    supportsTemplates: true,
    supportsImport: true,
  },
};

export const AgentTabs = [
  FlowType.Agent,
  FlowType.Flow,
  FlowType.Compiler,
] as const;

/**
 * Map canvas_category → FlowType for resolving icons in agent cards.
 */
export const CanvasCategoryToFlowType: Record<string, FlowType> = {
  [AgentCategory.AgentCanvas]: FlowType.Agent,
  [AgentCategory.DataflowCanvas]: FlowType.Flow,
  [AgentListItemType.CompilationTemplateGroup]: FlowType.Compiler,
};
