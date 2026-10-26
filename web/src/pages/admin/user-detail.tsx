import { useTranslation } from 'react-i18next';
import { useNavigate, useParams } from 'react-router';

import { LucideArrowLeft, LucideDot } from 'lucide-react';

import { useQuery } from '@tanstack/react-query';

import { Routes } from '@/routes';

import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import Spotlight from '@/components/spotlight';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardHeader } from '@/components/ui/card';

import { getUserDetails } from '@/services/admin-service';

import EnterpriseFeature from './components/enterprise-feature';
import { parseBooleanish } from './utils';

function AdminUserDetail() {
  const navigate = useNavigate();
  const { t } = useTranslation();
  const { id } = useParams();

  const { data: detail } = useQuery({
    queryKey: ['admin/userDetail', id],
    queryFn: async () => {
      const userDetails = await getUserDetails(id!);
      return userDetails.data.data[0];
    },
    enabled: !!id,
    retry: false,
  });

  return (
    <section className="px-10 py-5 size-full flex flex-col">
      <nav className="mb-5">
        <Button
          variant="outline"
          className="h-10 px-3 dark:bg-bg-input dark:border-border-button"
          onClick={() => navigate(`${Routes.AdminUserManagement}`)}
        >
          <LucideArrowLeft />
          <span>{t('admin.back')}</span>
        </Button>
      </nav>

      <Card className="!shadow-none relative h-0 basis-0 grow flex flex-col bg-transparent border-0.5 border-border-button overflow-hidden">
        <Spotlight />

        <CardHeader className="pb-10 border-b-0.5 dark:border-border-button space-y-8">
          <section className="flex items-center gap-4 text-base">
            <RAGFlowAvatar
              avatar={detail?.avatar}
              name={detail?.email}
              isPerson
            />

            <span>{detail?.email}</span>

            <Badge
              variant={
                parseBooleanish(detail?.is_active) ? 'success' : 'destructive'
              }
              className="pl-[.5em]"
            >
              <LucideDot className="size-[1em] stroke-[8] mr-1" />
              {t(
                parseBooleanish(detail?.is_active)
                  ? 'admin.active'
                  : 'admin.inactive',
              )}
            </Badge>

            <EnterpriseFeature>
              {() =>
                detail?.role && (
                  <Badge variant="secondary">{detail?.role}</Badge>
                )
              }
            </EnterpriseFeature>
          </section>

          <section className="flex items-start px-14 space-x-14">
            <div>
              <div className="text-sm text-text-secondary mb-2">
                {t('admin.lastLoginTime')}
              </div>
              <div>{detail?.last_login_time}</div>
            </div>

            <div>
              <div className="text-sm text-text-secondary mb-2">
                {t('admin.createTime')}
              </div>
              <div>{detail?.create_date}</div>
            </div>

            <div>
              <div className="text-sm text-text-secondary mb-2">
                {t('admin.lastUpdateTime')}
              </div>
              <div>{detail?.update_date}</div>
            </div>

            <div>
              <div className="text-sm text-text-secondary mb-2">
                {t('admin.language')}
              </div>
              <div>{detail?.language}</div>
            </div>

            <div>
              <div className="text-sm text-text-secondary mb-2">
                {t('admin.isAnonymous')}
              </div>
              <div>{t(detail?.is_anonymous ? 'admin.yes' : 'admin.no')}</div>
            </div>

            <div>
              <div className="text-sm text-text-secondary mb-2">
                {t('admin.isSuperuser')}
              </div>
              <div>{t(detail?.is_superuser ? 'admin.yes' : 'admin.no')}</div>
            </div>
          </section>
        </CardHeader>
      </Card>
    </section>
  );
}

export default AdminUserDetail;
