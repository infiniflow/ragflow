import { ReactComponent as LogoutIcon } from '@/assets/svg/logout.svg';
import { ReactComponent as ModelIcon } from '@/assets/svg/model-providers.svg';
import { ReactComponent as PasswordIcon } from '@/assets/svg/password.svg';
import { ReactComponent as ProfileIcon } from '@/assets/svg/profile.svg';
import { ReactComponent as TeamIcon } from '@/assets/svg/team.svg';
import { UserSettingRouteKey } from '@/constants/setting';
import { MonitorOutlined } from '@ant-design/icons';

export const UserSettingIconMap = {
  [UserSettingRouteKey.Profile]: <ProfileIcon />,
  [UserSettingRouteKey.Password]: <PasswordIcon />,
  [UserSettingRouteKey.Model]: <ModelIcon />,
  [UserSettingRouteKey.System]: <MonitorOutlined style={{ fontSize: 24 }} />,
  [UserSettingRouteKey.Team]: <TeamIcon />,
  [UserSettingRouteKey.Logout]: <LogoutIcon />,
};

export * from '@/constants/setting';

export const LocalLlmFactories = ['Ollama', 'Xinference','LocalAI','LM-Studio'];
