import { cn } from '@/lib/utils';
import { LucidePanelLeftClose } from 'lucide-react';
import { ComponentPropsWithoutRef } from 'react';
import { useTranslation } from 'react-i18next';
import { TruncatedText } from './truncated-text';
import { Button } from './ui/button';

export function SidebarHeader({
  className,
  children,
  ...props
}: ComponentPropsWithoutRef<'header'>) {
  return (
    <header
      className={cn('flex h-16 shrink-0 items-center gap-2', className)}
      {...props}
    >
      {children}
    </header>
  );
}

interface SecondarySidebarHeaderProps {
  title: string;
  onCollapse: () => void;
}

export function SecondarySidebarHeader({
  title,
  onCollapse,
}: SecondarySidebarHeaderProps) {
  const { t } = useTranslation();

  return (
    <SidebarHeader className="px-5">
      <TruncatedText
        as="h2"
        className="min-w-0 flex-1 truncate text-base font-normal text-text-primary"
        tooltip={title}
      >
        {title}
      </TruncatedText>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={t('sidebar.collapseSidebar')}
        title={t('sidebar.collapseSidebar')}
        onClick={onCollapse}
      >
        <LucidePanelLeftClose className="size-4" />
      </Button>
    </SidebarHeader>
  );
}
