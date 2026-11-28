import {
  SecondarySidebar,
  SecondarySidebarMenu,
} from '@/components/secondary-sidebar';
import { AgentQuery } from '@/constants/agent';
import { Routes } from '@/routes';
import {
  ClipboardClock,
  LucideFrame,
  LucideMessageSquareMore,
  LucideSettings,
} from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useMatch, useParams, useSearchParams } from 'react-router';
import { useIsPipeline } from '../hooks/use-is-pipeline';

enum AgentMenuKey {
  Canvas = 'canvas',
  VersionHistory = 'version-history',
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
  const isVersionHistory = Boolean(
    useMatch({ path: Routes.AgentVersionHistory, end: true }),
  );
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
      key: AgentMenuKey.VersionHistory,
      icon: <ClipboardClock className="size-4 shrink-0" />,
      label: t('flow.historyVersion'),
      path: `${Routes.Agent}/${id}/version-history`,
      active: isVersionHistory,
    });

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
      onSelect: onMenuSelect,
    }));
  }, [
    id,
    isCanvas,
    isLaunch,
    isPipeline,
    isSetting,
    isVersionHistory,
    onMenuSelect,
    searchParams,
    t,
  ]);

  return (
    <SecondarySidebar title={title} onCollapse={onCollapse}>
      <SecondarySidebarMenu items={items} navigationLabel={title} />
    </SecondarySidebar>
  );
}
