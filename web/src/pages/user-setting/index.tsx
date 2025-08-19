import { Flex } from 'antd';
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
    <section>
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
      <Flex className={cn(styles.settingWrapper, '-translate-y-6')}>
        <SideBar></SideBar>
        <Flex flex={1} className={styles.outletWrapper}>
          <Outlet></Outlet>
        </Flex>
      </Flex>
    </section>
  );
};

export default UserSetting;
