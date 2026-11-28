import { cn } from '@/lib/utils';
import type { ComponentPropsWithoutRef, ReactNode } from 'react';
import type { To } from 'react-router';
import { SecondarySidebarHeader } from './sidebar-header';
import { Button } from './ui/button';

type SecondarySidebarMenuItemBase = {
  key: string;
  icon: ReactNode;
  label: ReactNode;
  active: boolean;
  testId?: string;
};

export type SecondarySidebarMenuItem = SecondarySidebarMenuItemBase &
  ({ to: To; onSelect?: () => void } | { to?: never; onSelect: () => void });

type SecondarySidebarMenuProps = {
  items: SecondarySidebarMenuItem[];
  navigationLabel: string;
  className?: string;
};

function SecondarySidebarMenuButton({
  item,
}: {
  item: SecondarySidebarMenuItem;
}) {
  const className = cn(
    'relative h-10 justify-start gap-2.5 px-3 text-base',
    item.active && 'bg-bg-card text-text-primary',
  );
  const content = (
    <>
      {item.icon}
      <span className="min-w-0 truncate">{item.label}</span>
    </>
  );

  if (item.to !== undefined) {
    return (
      <Button
        asLink
        block
        variant="ghost"
        to={item.to}
        aria-current={item.active ? 'page' : undefined}
        className={className}
        onClick={item.onSelect}
        data-testid={item.testId}
      >
        {content}
      </Button>
    );
  }

  return (
    <Button
      type="button"
      block
      variant="ghost"
      aria-pressed={item.active}
      className={className}
      onClick={item.onSelect}
      data-testid={item.testId}
    >
      {content}
    </Button>
  );
}

export function SecondarySidebarMenu({
  items,
  navigationLabel,
  className,
}: SecondarySidebarMenuProps) {
  return (
    <nav
      className={cn('overflow-y-auto px-5 pb-5 pt-1', className)}
      aria-label={navigationLabel}
    >
      <ul className="space-y-5">
        {items.map((item) => (
          <li key={item.key}>
            <SecondarySidebarMenuButton item={item} />
          </li>
        ))}
      </ul>
    </nav>
  );
}

type SecondarySidebarProps = ComponentPropsWithoutRef<'aside'> & {
  title?: string;
  onCollapse?: () => void;
};

export function SecondarySidebar({
  title,
  onCollapse,
  className,
  children,
  ...props
}: SecondarySidebarProps) {
  return (
    <aside
      className={cn('relative flex w-64 shrink-0 flex-col', className)}
      {...props}
    >
      {title !== undefined && (
        <SecondarySidebarHeader title={title} onCollapse={onCollapse} />
      )}
      {children}
    </aside>
  );
}
