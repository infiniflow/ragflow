import { AgentCategory } from '@/constants/agent';
import { AgentListItemType } from '@/interfaces/database/agent';
import {
  Box,
  ChartPie,
  Component,
  LucideIcon,
  MessageCircleCode,
  Network,
  PencilRuler,
  Route,
  Shapes,
  Sparkle,
} from 'lucide-react';

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
    supportsTemplates: false,
    supportsImport: true,
  },
};

export const AgentTabs = [
  FlowType.Agent,
  FlowType.Flow,
  FlowType.Compiler,
] as const;

export enum TemplateMenuItemKey {
  Recommended = 'Recommended',
  Agent = 'Agent',
  CustomerSupport = 'Customer Support',
  Marketing = 'Marketing',
  ConsumerApp = 'Consumer App',
  Other = 'Other',
}

export const TemplateMenuItems = [
  {
    icon: Sparkle,
    labelKey: 'recommended',
    key: TemplateMenuItemKey.Recommended,
  },
  {
    icon: Box,
    labelKey: 'agent',
    key: TemplateMenuItemKey.Agent,
  },
  {
    icon: MessageCircleCode,
    labelKey: 'customerSupport',
    key: TemplateMenuItemKey.CustomerSupport,
  },
  {
    icon: ChartPie,
    labelKey: 'marketing',
    key: TemplateMenuItemKey.Marketing,
  },
  {
    icon: Component,
    labelKey: 'consumerApp',
    key: TemplateMenuItemKey.ConsumerApp,
  },
  {
    icon: PencilRuler,
    labelKey: 'other',
    key: TemplateMenuItemKey.Other,
  },
] as const;

/**
 * Map canvas_category → FlowType for resolving icons in agent cards.
 */
export const CanvasCategoryToFlowType: Record<string, FlowType> = {
  [AgentCategory.AgentCanvas]: FlowType.Agent,
  [AgentCategory.DataflowCanvas]: FlowType.Flow,
  [AgentListItemType.CompilationTemplateGroup]: FlowType.Compiler,
};
