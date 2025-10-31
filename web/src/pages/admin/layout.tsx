import { Button } from '@/components/ui/button';
import message from '@/components/ui/message';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import adminService from '@/services/admin-service';
import authorizationUtil from '@/utils/authorization-util';
import { useMutation } from '@tanstack/react-query';
import {
  LucideMonitor,
  LucideServerCrash,
  LucideSquareUserRound,
  LucideUserCog,
  LucideUserStar,
} from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { NavLink, Outlet, useNavigate } from 'umi';
import ThemeSwitch from './components/theme-switch';
import { IS_ENTERPRISE } from './utils';

const AdminLayout = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();

  const navItems = useMemo(
    () => [
      {
        path: Routes.AdminServices,
        name: t('admin.serviceStatus'),
        icon: <LucideServerCrash className="size-[1em]" />,
      },
      {
        path: Routes.AdminUserManagement,
        name: t('admin.userManagement'),
        icon: <LucideUserCog className="size-[1em]" />,
      },
      ...(IS_ENTERPRISE
        ? [
            {
              path: Routes.AdminWhitelist,
              name: t('admin.registrationWhitelist'),
              icon: <LucideUserStar className="size-[1em]" />,
            },
            {
              path: Routes.AdminRoles,
              name: t('admin.roles'),
              icon: <LucideSquareUserRound className="size-[1em]" />,
            },
            {
              path: Routes.AdminMonitoring,
              name: t('admin.monitoring'),
              icon: <LucideMonitor className="size-[1em]" />,
            },
          ]
        : []),
    ],
    [t],
  );

  const logoutMutation = useMutation({
    mutationKey: ['adminLogout'],
    mutationFn: async () => {
      await adminService.logout();

      message.success(t('message.logout'));
      authorizationUtil.removeAll();
      navigate(Routes.Admin);
    },
    retry: false,
  });

  return (
    <main className="w-screen h-screen flex flex-row px-6 pt-12 pb-6 dark:*:focus-visible:ring-white">
      <aside className="w-72 mr-6 flex flex-col gap-6">
        <div className="flex items-center mb-6">
          <img className="size-8 mr-5" src="/logo.svg" alt="logo" />
          <span className="text-xl font-bold">{t('admin.title')}</span>
        </div>

        <nav>
          <ul className="space-y-4">
            {navItems.map((it) => (
              <li key={it.path}>
                <NavLink
                  to={it.path}
                  className={({ isActive }) =>
                    cn(
                      'px-4 py-3 rounded-lg',
                      'text-base w-full flex items-center justify-start text-text-secondary',
                      'hover:bg-bg-card focus:bg-bg-card focus-visible:bg-bg-card',
                      'hover:text-text-primary focus:text-text-primary focus-visible:text-text-primary',
                      'active:text-text-primary',
                      {
                        'bg-bg-card text-text-primary': isActive,
                      },
                    )
                  }
                >
                  {it.icon}
                  <span className="ml-3">{it.name}</span>
                </NavLink>
              </li>
            ))}
          </ul>
        </nav>

        <div className="mt-auto space-y-4">
          <div className="text-right">
            <ThemeSwitch />
          </div>

          <Button
            size="lg"
            variant="transparent"
            className="block w-full dark:border-border-button"
            onClick={() => logoutMutation.mutate()}
          >
            {t('header.logout')}
          </Button>
        </div>
      </aside>

      <section className="flex-1 h-full">
        <Outlet />
      </section>
    </main>
  );
};

export default AdminLayout;
