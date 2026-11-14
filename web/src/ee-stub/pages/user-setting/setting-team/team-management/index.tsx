import UserSettingTeam from '@/pages/user-setting/setting-team/legacy-page';

/**
 * Enterprise slot for the team management page. The enterprise build replaces
 * this file with the department/group/member tabs; open source keeps the
 * upstream team page.
 */
export function TeamManagement() {
  return <UserSettingTeam />;
}
