import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { LucideEye, LucideEyeOff } from 'lucide-react';

import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import message from '@/components/ui/message';
import { Switch } from '@/components/ui/switch';
import {
  NavigationSections,
  type NavigationSection,
} from '@/constants/navigation';
import { SystemConfigKeys } from '@/hooks/use-system-request';
import {
  getNavigationVisibility,
  setNavigationVisibility,
} from '@/services/admin-service';

const NavigationVisibilityKeys = {
  detail: ['admin', 'navigation-visibility'] as const,
};

const SectionLabels: Record<NavigationSection, string> = {
  home: 'header.home',
  dataset: 'header.dataset',
  chat: 'header.chat',
  search: 'header.search',
  agent: 'header.flow',
  memory: 'header.memories',
  catalog: 'header.openMetadata',
  business_documents: 'header.businessDocuments',
  file_manager: 'header.fileManager',
};

function AdminNavigationVisibility() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [visibleSections, setVisibleSections] = useState<NavigationSection[]>(
    [],
  );

  const { data, isLoading } = useQuery({
    queryKey: NavigationVisibilityKeys.detail,
    queryFn: async () => (await getNavigationVisibility()).data.data,
  });

  useEffect(() => {
    if (data) {
      setVisibleSections(data.visible_sections);
    }
  }, [data]);

  const selected = useMemo(() => new Set(visibleSections), [visibleSections]);
  const isDirty = useMemo(() => {
    if (!data) {
      return false;
    }

    return NavigationSections.some(
      (section) =>
        selected.has(section) !== data.visible_sections.includes(section),
    );
  }, [data, selected]);

  const mutation = useMutation({
    mutationFn: setNavigationVisibility,
    onSuccess: async ({ data: response }) => {
      setVisibleSections(response.data.visible_sections);
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: NavigationVisibilityKeys.detail,
        }),
        queryClient.invalidateQueries({ queryKey: SystemConfigKeys.all }),
      ]);
      message.success(t('admin.navigationVisibilityPage.saved'));
    },
  });

  const setSectionVisible = (section: NavigationSection, visible: boolean) => {
    setVisibleSections((current) =>
      visible
        ? NavigationSections.filter(
            (candidate) => candidate === section || current.includes(candidate),
          )
        : current.filter((candidate) => candidate !== section),
    );
  };

  return (
    <Card
      className="h-full overflow-y-auto rounded-xl border-0.5 border-border-button bg-transparent !shadow-none"
      data-testid="navigation-visibility-admin"
    >
      <CardHeader className="border-b border-border-button">
        <CardTitle>{t('admin.navigationVisibility')}</CardTitle>
        <CardDescription>
          {t('admin.navigationVisibilityPage.description')}
        </CardDescription>
      </CardHeader>

      <CardContent className="space-y-6 pt-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <span className="text-sm text-text-secondary">
            {t('admin.navigationVisibilityPage.visibleCount', {
              count: visibleSections.length,
              total: NavigationSections.length,
            })}
          </span>
          <div className="flex gap-2">
            <Button
              variant="outline"
              data-testid="navigation-visibility-show-all"
              onClick={() => setVisibleSections([...NavigationSections])}
              disabled={isLoading || mutation.isPending}
            >
              <LucideEye className="mr-2 size-4" />
              {t('admin.navigationVisibilityPage.showAll')}
            </Button>
            <Button
              variant="outline"
              data-testid="navigation-visibility-hide-all"
              onClick={() => setVisibleSections([])}
              disabled={isLoading || mutation.isPending}
            >
              <LucideEyeOff className="mr-2 size-4" />
              {t('admin.navigationVisibilityPage.hideAll')}
            </Button>
          </div>
        </div>

        <div className="grid gap-3 lg:grid-cols-2">
          {NavigationSections.map((section) => {
            const label = t(SectionLabels[section]);
            return (
              <label
                key={section}
                className="flex cursor-pointer items-center justify-between gap-4 rounded-lg border border-border-button bg-bg-card px-4 py-4"
              >
                <span className="font-medium">{label}</span>
                <Switch
                  data-testid={`navigation-section-${section}`}
                  checked={selected.has(section)}
                  onCheckedChange={(checked) =>
                    setSectionVisible(section, checked)
                  }
                  disabled={isLoading || mutation.isPending}
                  aria-label={label}
                />
              </label>
            );
          })}
        </div>

        <div className="flex justify-end">
          <Button
            data-testid="navigation-visibility-save"
            onClick={() => mutation.mutate(visibleSections)}
            disabled={!isDirty || mutation.isPending}
          >
            {mutation.isPending
              ? t('admin.navigationVisibilityPage.saving')
              : t('admin.navigationVisibilityPage.save')}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

export default AdminNavigationVisibility;
