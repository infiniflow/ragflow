import { TruncatedText } from '@/components/truncated-text';
import { Button } from '@/components/ui/button';
import { AgentQuery } from '@/constants/agent';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import {
  LucideFrame,
  LucideMessageSquareMore,
  LucidePanelLeftClose,
  LucideSettings,
} from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useMatch, useParams, useSearchParams } from 'react-router';
import { useIsPipeline } from '../hooks/use-is-pipeline';

enum AgentMenuKey {
  Canvas = 'canvas',
  Launch = 'launch',
  Setting = 'setting',
}

type AgentSidebarProps = {
  title: string;
  onCollapse: () => void;
  onMenuSelect: () => void;
};

export function AgentSidebar({
  title,
  onCollapse,
  onMenuSelect,
}: AgentSidebarProps) {
  const { t } = useTranslation();
  const { id } = useParams();
  const [searchParams] = useSearchParams();
  const isPipeline = useIsPipeline();
  const isCanvas = Boolean(
    useMatch({ path: `${Routes.Agent}/:id`, end: true }),
  );
  const isLaunch = Boolean(useMatch({ path: Routes.AgentExplore, end: true }));
  const isSetting = Boolean(useMatch({ path: Routes.AgentSetting, end: true }));

  const items = useMemo(() => {
    const category = searchParams.get(AgentQuery.Category);
    const search = category
      ? `?${new URLSearchParams({ [AgentQuery.Category]: category }).toString()}`
      : '';
    const list = [
      {
        key: AgentMenuKey.Canvas,
        icon: <LucideFrame className="size-4 shrink-0" />,
        label: t('flow.canvas'),
        path: `${Routes.Agent}/${id}`,
        active: isCanvas,
      },
    ];

    if (!isPipeline) {
      list.push({
        key: AgentMenuKey.Launch,
        icon: <LucideMessageSquareMore className="size-4 shrink-0" />,
        label: t('explore.title'),
        path: `${Routes.Agent}/${id}/explore`,
        active: isLaunch,
      });
    }

    list.push({
      key: AgentMenuKey.Setting,
      icon: <LucideSettings className="size-4 shrink-0" />,
      label: t('flow.setting'),
      path: `${Routes.Agent}/${id}/setting`,
      active: isSetting,
    });

    return list.map((item) => ({
      ...item,
      to: { pathname: item.path, search },
    }));
  }, [id, isCanvas, isLaunch, isPipeline, isSetting, searchParams, t]);

  return (
    <aside className="flex w-64 flex-col">
      <header className="flex items-center gap-2 px-5 pb-4">
        <TruncatedText
          as="h3"
          className="min-w-0 flex-1 truncate text-lg font-semibold text-text-primary"
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
      </header>

      <nav className="overflow-y-auto px-5 pb-5 pt-1">
        <ul className="space-y-2">
          {items.map((item) => (
            <li key={item.key}>
              <Button
                asLink
                block
                variant="ghost"
                to={item.to}
                aria-current={item.active ? 'page' : undefined}
                className={cn(
                  'relative h-10 justify-start gap-2.5 px-3 text-base',
                  item.active && 'bg-bg-card text-text-primary',
                )}
                onClick={onMenuSelect}
              >
                {item.icon}
                <span>{item.label}</span>
              </Button>
            </li>
          ))}
        </ul>
      </nav>
    </aside>
  );
}
