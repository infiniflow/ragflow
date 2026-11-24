import ListFilterBar from '@/components/list-filter-bar';
import type { ComponentProps } from 'react';
import { NavGroupTabs } from './nav-group-tabs';

type NavGroupListFilterBarProps = Omit<
  ComponentProps<typeof ListFilterBar>,
  'title' | 'icon' | 'iconClassName'
>;

export function NavGroupListFilterBar({
  leftPanel,
  ...props
}: NavGroupListFilterBarProps) {
  return <ListFilterBar {...props} leftPanel={leftPanel ?? <NavGroupTabs />} />;
}
