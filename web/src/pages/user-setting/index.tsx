import { Flex } from 'antd';
import { Outlet } from 'umi';
import SideBar from './sidebar';

const UserSetting = () => {
  return (
    <Flex>
      <SideBar></SideBar>
      <Outlet></Outlet>
    </Flex>
  );
};

export default UserSetting;
