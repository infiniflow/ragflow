import { Outlet } from 'umi';
import SideBar from './sidebar';

import { PageHeader } from '@/components/page-header';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { cn } from '@/lib/utils';
import { House } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import styles from './index.less';

const UserSetting = () => {
  const { t } = useTranslation();
  const { navigateToHome } = useNavigatePage();

  return (
    <section className="flex flex-col h-full">
      <PageHeader>
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink onClick={navigateToHome}>
                <House className="size-4" />
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>{t('setting.profile')}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
      </PageHeader>
      <div
        className={cn(styles.settingWrapper, 'overflow-auto flex flex-1 pt-4')}
      >
        <SideBar></SideBar>
        <div className={cn(styles.outletWrapper, 'flex flex-1 px-8')}>
          <Outlet></Outlet>
        </div>
      </div>
    </section>
  );
};

export default UserSetting;
