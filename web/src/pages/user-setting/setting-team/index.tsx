import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import {
  useFetchUserInfo,
  useListTenantUser,
} from '@/hooks/user-setting-hooks';
import { useTranslation } from 'react-i18next';

import Spotlight from '@/components/spotlight';
import { SearchInput } from '@/components/ui/input';
import { Separator } from '@/components/ui/separator';
import { UserPlus } from 'lucide-react';
import { useState } from 'react';
import AddingUserModal from './add-user-modal';
import { useAddUser } from './hooks';
import TenantTable from './tenant-table';
import UserTable from './user-table';

const UserSettingTeam = () => {
  const { data: userInfo } = useFetchUserInfo();
  const { t } = useTranslation();
  const [searchTerm, setSearchTerm] = useState('');
  const [searchUser, setSearchUser] = useState('');
  useListTenantUser();
  const {
    addingTenantModalVisible,
    hideAddingTenantModal,
    showAddingTenantModal,
    handleAddUserOk,
  } = useAddUser();

  return (
    <div className="w-full flex flex-col gap-4 p-4 relative">
      <Spotlight />
      <Card className="bg-transparent border-none px-0">
        <CardHeader className="flex flex-row items-center justify-between space-y-0 px-0 pt-1">
          <CardTitle className="text-2xl font-medium">
            {userInfo?.nickname} {t('setting.workspace')}
          </CardTitle>
        </CardHeader>
      </Card>
      <Separator className="border-border-button bg-border-button w-[calc(100%+2rem)] -translate-x-4 -translate-y-4" />
      <Card className="bg-transparent border-none">
        <CardHeader className="flex flex-row items-center justify-between space-y-0 p-0 pb-4">
          {/* <User className="mr-2 h-5 w-5 text-[#1677ff]" /> */}
          <CardTitle className="text-base">
            {t('setting.teamMembers')}
          </CardTitle>
          <section className="flex gap-4 items-center">
            <SearchInput
              className="bg-bg-input border-border-default w-32"
              placeholder={t('common.search')}
              value={searchUser}
              onChange={(e) => setSearchUser(e.target.value)}
            />
            <Button onClick={showAddingTenantModal}>
              <UserPlus className=" h-4 w-4" />
              {t('setting.invite')}
            </Button>
          </section>
        </CardHeader>
        <CardContent className="p-0">
          <UserTable searchUser={searchUser}></UserTable>
        </CardContent>
      </Card>

      <Card className="bg-transparent border-none mt-8">
        <CardHeader className="flex flex-row items-center justify-between space-y-0 p-0 pb-4">
          {/* <Users className="mr-2 h-5 w-5 text-[#1677ff]" /> */}
          <CardTitle className="text-base">
            {t('setting.joinedTeams')}
          </CardTitle>
          <SearchInput
            className="bg-bg-input border-border-default w-32"
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            placeholder={t('common.search')}
          />
        </CardHeader>
        <CardContent className="p-0">
          <TenantTable searchTerm={searchTerm}></TenantTable>
        </CardContent>
      </Card>

      {addingTenantModalVisible && (
        <AddingUserModal
          visible
          hideModal={hideAddingTenantModal}
          onOk={handleAddUserOk}
        ></AddingUserModal>
      )}
    </div>
  );
};

export default UserSettingTeam;
