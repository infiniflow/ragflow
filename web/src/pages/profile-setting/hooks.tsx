import { ProfileSettingRouteKey } from '@/constants/setting';
import { useSecondPathName } from '@/hooks/route-hook';

export const useGetPageTitle = (): string => {
  const pathName = useSecondPathName();

  const LabelMap = {
    [ProfileSettingRouteKey.Profile]: 'User profile',
    [ProfileSettingRouteKey.Plan]: 'Plan & balance',
    [ProfileSettingRouteKey.Model]: 'Model management',
    [ProfileSettingRouteKey.System]: 'System',
    [ProfileSettingRouteKey.Api]: 'Api',
    [ProfileSettingRouteKey.Team]: 'Team management',
    [ProfileSettingRouteKey.Prompt]: 'Prompt management',
    [ProfileSettingRouteKey.Chunk]: 'Chunking method',
    [ProfileSettingRouteKey.Logout]: 'Logout',
  };

  return LabelMap[pathName as ProfileSettingRouteKey];
};
