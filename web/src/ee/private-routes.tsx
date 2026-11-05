import type { ComponentType, ReactNode } from 'react';

/**
 * Enterprise seam for the private route table. The enterprise build replaces
 * this file with the enterprise routes merged over the upstream tree; open
 * source only needs the path identifiers that shared code navigates to.
 */
export enum PrivateRoutes {
  Price = '/price',
  Billing = '/billing',
  EvaluationDetail = '/evaluation-detail',
  Evaluation = '/evaluation',
  Tickets = '/support',
  TicketCreate = '/support/create',
  TicketDetail = '/support/:id',
  AdminRoleConfigModelProviders = '/admin/roles/:roleName/model-providers',
  AdminModelUsageStatistics = '/admin/model-usage-statistics',
  AdminSettings = '/admin/settings',
  AdminSSOProviders = '/admin/sso',
  AdminModelProviders = '/admin/model-providers',
}

export type PrivateRoute = {
  path?: string;
  Component?: () => Promise<{ default: ComponentType<any> }>;
  errorElement?: ReactNode;
  children?: PrivateRoute[];
  replace?: string[];
  matchIndex?: number;
};

export const privateRoutes: PrivateRoute[] = [];
