import { UserSettingRouteKey } from '@/constants/setting';
import {
  ContainerOutlined,
  DesktopOutlined,
  PieChartOutlined,
} from '@ant-design/icons';

export const UserSettingIconMap = {
  [UserSettingRouteKey.Profile]: <PieChartOutlined />,
  [UserSettingRouteKey.Password]: <DesktopOutlined />,
  [UserSettingRouteKey.Model]: <ContainerOutlined />,
  [UserSettingRouteKey.Team]: <ContainerOutlined />,
  [UserSettingRouteKey.Logout]: <ContainerOutlined />,
};

export * from '@/constants/setting';
