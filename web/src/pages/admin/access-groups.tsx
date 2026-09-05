import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { LucidePencil, LucidePlus, LucideTrash2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import message from '@/components/ui/message';
import { Textarea } from '@/components/ui/textarea';
import type { NavigationSection } from '@/constants/navigation';
import {
  createAccessGroup,
  deleteAccessGroup,
  getAccessGroupOptions,
  listAccessGroups,
  updateAccessGroup,
} from '@/services/admin-service';

const emptyGroup = (): AdminService.AccessGroupInput => ({
  name: '',
  description: '',
  user_ids: [],
  dataset_ids: [],
  sections: [],
});

const sectionLabels: Record<NavigationSection, string> = {
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

function toggle(values: string[], value: string, checked: boolean) {
  return checked ? [...values, value] : values.filter((item) => item !== value);
}

export default function AdminAccessGroups() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [editingId, setEditingId] = useState<string | null>(null);
  const [draft, setDraft] = useState<AdminService.AccessGroupInput>(emptyGroup);
  const groupsQuery = useQuery({
    queryKey: ['admin', 'access-groups'],
    queryFn: async () => (await listAccessGroups()).data.data.items,
  });
  const optionsQuery = useQuery({
    queryKey: ['admin', 'access-groups', 'options'],
    queryFn: async () => (await getAccessGroupOptions()).data.data,
  });

  useEffect(() => {
    if (
      editingId &&
      !groupsQuery.data?.some((group) => group.id === editingId)
    ) {
      setEditingId(null);
      setDraft(emptyGroup());
    }
  }, [editingId, groupsQuery.data]);

  const saveMutation = useMutation({
    mutationFn: () =>
      editingId
        ? updateAccessGroup(editingId, draft)
        : createAccessGroup(draft),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: ['admin', 'access-groups'],
      });
      setEditingId(null);
      setDraft(emptyGroup());
      message.success(t('admin.accessGroupsPage.saved'));
    },
  });
  const deleteMutation = useMutation({
    mutationFn: deleteAccessGroup,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ['admin', 'access-groups'] }),
  });

  const edit = (group: AdminService.AccessGroup) => {
    setEditingId(group.id);
    setDraft({
      name: group.name,
      description: group.description ?? '',
      user_ids: [...group.user_ids],
      dataset_ids: [...group.dataset_ids],
      sections: [...group.sections],
    });
  };

  return (
    <div className="grid h-full min-h-0 gap-4 overflow-y-auto xl:grid-cols-[minmax(20rem,0.8fr)_minmax(34rem,1.2fr)]">
      <Card className="bg-transparent !shadow-none">
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle>{t('admin.accessGroups')}</CardTitle>
          <Button
            size="sm"
            onClick={() => {
              setEditingId(null);
              setDraft(emptyGroup());
            }}
          >
            <LucidePlus className="mr-2 size-4" />
            {t('admin.accessGroupsPage.new')}
          </Button>
        </CardHeader>
        <CardContent className="space-y-3">
          {groupsQuery.data?.map((group) => (
            <div
              key={group.id}
              className="rounded-lg border border-border-button bg-bg-card p-4"
            >
              <div className="flex items-start justify-between gap-3">
                <div>
                  <div className="font-medium">{group.name}</div>
                  <div className="text-sm text-text-secondary">
                    {group.description}
                  </div>
                  <div className="mt-2 text-xs text-text-secondary">
                    {t('admin.accessGroupsPage.summary', {
                      users: group.user_ids.length,
                      datasets: group.dataset_ids.length,
                      sections: group.sections.length,
                    })}
                  </div>
                </div>
                <div className="flex">
                  <Button
                    size="icon"
                    variant="ghost"
                    aria-label={t('admin.edit')}
                    onClick={() => edit(group)}
                  >
                    <LucidePencil className="size-4" />
                  </Button>
                  <Button
                    size="icon"
                    variant="ghost"
                    aria-label={t('admin.delete')}
                    onClick={() => {
                      if (
                        window.confirm(
                          t('admin.accessGroupsPage.deleteConfirm'),
                        )
                      )
                        deleteMutation.mutate(group.id);
                    }}
                  >
                    <LucideTrash2 className="size-4" />
                  </Button>
                </div>
              </div>
            </div>
          ))}
        </CardContent>
      </Card>

      <Card className="bg-transparent !shadow-none">
        <CardHeader>
          <CardTitle>
            {editingId
              ? t('admin.accessGroupsPage.edit')
              : t('admin.accessGroupsPage.create')}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="space-y-2">
            <Label htmlFor="access-group-name">
              {t('admin.accessGroupsPage.name')}
            </Label>
            <Input
              id="access-group-name"
              value={draft.name}
              onChange={(event) =>
                setDraft({ ...draft, name: event.target.value })
              }
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="access-group-description">
              {t('admin.accessGroupsPage.description')}
            </Label>
            <Textarea
              id="access-group-description"
              value={draft.description}
              onChange={(event) =>
                setDraft({ ...draft, description: event.target.value })
              }
            />
          </div>

          <fieldset className="space-y-3">
            <legend className="font-medium">
              {t('admin.accessGroupsPage.users')}
            </legend>
            <div className="grid gap-2 md:grid-cols-2">
              {optionsQuery.data?.users
                .filter((user) => !user.is_superuser)
                .map((user) => (
                  <Label
                    key={user.id}
                    className="flex items-center gap-2 rounded-md border border-border-button p-3"
                  >
                    <Checkbox
                      checked={draft.user_ids.includes(user.id)}
                      onCheckedChange={(checked) =>
                        setDraft({
                          ...draft,
                          user_ids: toggle(
                            draft.user_ids,
                            user.id,
                            checked === true,
                          ),
                        })
                      }
                    />
                    <span>{user.nickname || user.email}</span>
                  </Label>
                ))}
            </div>
          </fieldset>
          <fieldset className="space-y-3">
            <legend className="font-medium">
              {t('admin.accessGroupsPage.datasets')}
            </legend>
            <div className="grid max-h-52 gap-2 overflow-y-auto md:grid-cols-2">
              {optionsQuery.data?.datasets.map((dataset) => (
                <Label
                  key={dataset.id}
                  className="flex items-center gap-2 rounded-md border border-border-button p-3"
                >
                  <Checkbox
                    checked={draft.dataset_ids.includes(dataset.id)}
                    onCheckedChange={(checked) =>
                      setDraft({
                        ...draft,
                        dataset_ids: toggle(
                          draft.dataset_ids,
                          dataset.id,
                          checked === true,
                        ),
                      })
                    }
                  />
                  <span>{dataset.name}</span>
                </Label>
              ))}
            </div>
          </fieldset>
          <fieldset className="space-y-3">
            <legend className="font-medium">
              {t('admin.accessGroupsPage.sections')}
            </legend>
            <div className="grid gap-2 md:grid-cols-2">
              {optionsQuery.data?.sections.map((section) => (
                <Label
                  key={section}
                  className="flex items-center gap-2 rounded-md border border-border-button p-3"
                >
                  <Checkbox
                    checked={draft.sections.includes(section)}
                    onCheckedChange={(checked) =>
                      setDraft({
                        ...draft,
                        sections: toggle(
                          draft.sections,
                          section,
                          checked === true,
                        ) as NavigationSection[],
                      })
                    }
                  />
                  <span>{t(sectionLabels[section])}</span>
                </Label>
              ))}
            </div>
          </fieldset>
          <div className="flex justify-end">
            <Button
              disabled={!draft.name.trim() || saveMutation.isPending}
              onClick={() => saveMutation.mutate()}
            >
              {t('admin.accessGroupsPage.save')}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
