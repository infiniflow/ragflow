export const UserSettingBaseKey = 'user-setting';

export enum UserSettingRouteKey {
  Profile = 'profile',
  Password = 'password',
  Model = 'model',
  Team = 'team',
  Logout = 'logout',
}

export const UserSettingRouteMap = {
  [UserSettingRouteKey.Profile]: 'Profile',
  [UserSettingRouteKey.Password]: 'Password',
  [UserSettingRouteKey.Model]: 'Model Providers',
  [UserSettingRouteKey.Team]: 'Team',
  [UserSettingRouteKey.Logout]: 'Log out',
};
