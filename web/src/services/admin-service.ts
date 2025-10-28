import { message, notification } from 'antd';
import axios from 'axios';
import { Navigate } from 'umi';

import { Authorization } from '@/constants/authorization';
import i18n from '@/locales/config';
import { Routes } from '@/routes';
import api from '@/utils/api';
import authorizationUtil, {
  getAuthorization,
} from '@/utils/authorization-util';
import { convertTheKeysOfTheObjectToSnake } from '@/utils/common-util';
import { ResultCode, RetcodeMessage } from '@/utils/request';

const request = axios.create({
  timeout: 300000,
});

request.interceptors.request.use((config) => {
  const data = convertTheKeysOfTheObjectToSnake(config.data);
  const params = convertTheKeysOfTheObjectToSnake(config.params) as any;

  const newConfig = { ...config, data, params };

  // @ts-ignore
  if (!newConfig.skipToken) {
    newConfig.headers.set(Authorization, getAuthorization());
  }

  return newConfig;
});

request.interceptors.response.use(
  (response) => {
    if (response.config.responseType === 'blob') {
      return response;
    }

    const { data } = response ?? {};

    if (data?.code === 100) {
      message.error(data?.message);
    } else if (data?.code === 401) {
      notification.error({
        message: data?.message,
        description: data?.message,
        duration: 3,
      });

      authorizationUtil.removeAll();
      Navigate({ to: Routes.Admin });
    } else if (data?.code && data.code !== 0) {
      notification.error({
        message: `${i18n.t('message.hint')}: ${data?.code}`,
        description: data?.message,
        duration: 3,
      });
    }

    return response;
  },
  (error) => {
    const { response, message } = error;
    const { data } = response ?? {};

    if (error.message === 'Failed to fetch') {
      notification.error({
        description: i18n.t('message.networkAnomalyDescription'),
        message: i18n.t('message.networkAnomaly'),
      });
    } else if (data?.code === 100) {
      message.error(data?.message);
    } else if (data?.code === 401) {
      notification.error({
        message: data?.message,
        description: data?.message,
        duration: 3,
      });

      authorizationUtil.removeAll();
      Navigate({ to: Routes.Admin });
    } else if (data?.code && data.code !== 0) {
      notification.error({
        message: `${i18n.t('message.hint')}: ${data?.code}`,
        description: data?.message,
        duration: 3,
      });
    } else if (response.status) {
      notification.error({
        message: `${i18n.t('message.requestError')} ${response.status}: ${response.config.url}`,
        description:
          RetcodeMessage[response.status as ResultCode] || response.statusText,
      });
    } else if (response.status === 413 || response?.status === 504) {
      message.error(RetcodeMessage[response?.status as ResultCode]);
    } else if (response.status === 401) {
      notification.error({
        message: response.data.message,
        description: response.data.message,
        duration: 3,
      });
      authorizationUtil.removeAll();
      window.location.href = location.origin + '/admin';
    }

    return error;
  },
);

const {
  adminLogin,
  adminLogout,
  adminListUsers,
  adminCreateUser,
  adminGetUserDetails: adminShowUserDetails,
  adminUpdateUserStatus,
  adminUpdateUserPassword,
  adminDeleteUser,
  adminListUserDatasets,
  adminListUserAgents,

  adminListServices,
  adminShowServiceDetails,

  adminListRoles,
  adminListRolesWithPermission,
  adminCreateRole,
  adminDeleteRole,
  adminUpdateRoleDescription,
  adminGetRolePermissions,
  adminAssignRolePermissions,
  adminRevokeRolePermissions,

  adminGetUserPermissions,
  adminUpdateUserRole,

  adminListResources,
} = api;

type ResponseData<D = {}> = {
  code: number;
  message: string;
  data: D;
};

export namespace AdminService {
  export type LoginData = {
    access_token: string;
    avatar: unknown;
    color_schema: 'Bright' | 'Dark';
    create_date: string;
    create_time: number;
    email: string;
    id: string;
    is_active: '0' | '1';
    is_anonymous: '0' | '1';
    is_authenticated: '0' | '1';
    is_superuser: boolean;
    language: string;
    last_login_time: string;
    login_channel: unknown;
    nickname: string;
    password: string;
    status: '0' | '1';
    timezone: string;
    update_date: [string];
    update_time: [number];
  };

  export type ListUsersItem = {
    create_date: string;
    email: string;
    is_active: '0' | '1';
    is_superuser: boolean;
    role: string;
    nickname: string;
  };

  export type UserDetail = {
    create_date: string;
    email: string;
    is_active: '0' | '1';
    is_anonymous: '0' | '1';
    is_superuser: boolean;
    language: string;
    last_login_time: string;
    login_channel: unknown;
    status: '0' | '1';
    update_date: string;
    role: string;
  };

  export type ListUserDatasetItem = {
    chunk_num: number;
    create_date: string;
    doc_num: number;
    language: string;
    name: string;
    permission: string;
    status: '0' | '1';
    token_num: number;
    update_date: string;
  };

  export type ListUserAgentItem = {
    canvas_category: 'agent';
    permission: 'string';
    title: string;
  };

  export type ListServicesItem = {
    extra: Record<string, unknown>;
    host: string;
    id: number;
    name: string;
    port: number;
    service_type: string;
    status: 'alive' | 'timeout' | 'fail';
  };

  export type ServiceDetail = {
    service_name: string;
    status: 'alive' | 'timeout';
    message: string | Record<string, any> | Record<string, any>[];
  };

  export type PermissionData = {
    enable: boolean;
    read: boolean;
    write: boolean;
    share: boolean;
  };

  export type ListRoleItem = {
    id: string;
    role_name: string;
    description: string;
    create_date: string;
    update_date: string;
  };

  export type ListRoleItemWithPermission = ListRoleItem & {
    permissions: Record<string, PermissionData>;
  };

  export type RoleDetailWithPermission = {
    role: {
      id: string;
      name: string;
      description: string;
    };
    permissions: Record<string, PermissionData>;
  };

  export type RoleDetail = {
    id: string;
    name: string;
    descrtiption: string;
    create_date: string;
    update_date: string;
  };

  export type AssignRolePermissionInput = {
    permissions: Record<string, Partial<PermissionData>>;
  };

  export type RevokeRolePermissionInput = AssignRolePermissionInput;

  export type UserDetailWithPermission = {
    user: {
      id: string;
      username: string;
      role: string;
    };
    role_permissions: Record<string, PermissionData>;
  };

  export type ResourceType = {
    resource_types: string[];
  };
}

export const login = (params: { email: string; password: string }) =>
  request.post<ResponseData<AdminService.LoginData>>(adminLogin, params);
export const logout = () => request.get<ResponseData<boolean>>(adminLogout);
export const listUsers = () =>
  request.get<ResponseData<AdminService.ListUsersItem[]>>(adminListUsers, {});

export const createUser = (email: string, password: string) =>
  request.post<ResponseData<boolean>>(adminCreateUser, {
    username: email,
    password,
  });
export const getUserDetails = (email: string) =>
  request.get<ResponseData<[AdminService.UserDetail]>>(
    adminShowUserDetails(email),
  );
export const listUserDatasets = (email: string) =>
  request.get<ResponseData<AdminService.ListUserDatasetItem[]>>(
    adminListUserDatasets(email),
  );
export const listUserAgents = (email: string) =>
  request.get<ResponseData<AdminService.ListUserAgentItem[]>>(
    adminListUserAgents(email),
  );
export const updateUserStatus = (email: string, status: 'on' | 'off') =>
  request.put(adminUpdateUserStatus(email), { activate_status: status });
export const updateUserPassword = (email: string, password: string) =>
  request.put(adminUpdateUserPassword(email), { new_password: password });
export const deleteUser = (email: string) =>
  request.delete(adminDeleteUser(email));

export const listServices = () =>
  request.get<ResponseData<AdminService.ListServicesItem[]>>(adminListServices);
export const showServiceDetails = (serviceId: number) =>
  request.get<ResponseData<AdminService.ServiceDetail>>(
    adminShowServiceDetails(String(serviceId)),
  );

export const createRole = (params: { roleName: string; description: string }) =>
  request.post<ResponseData<AdminService.RoleDetail>>(adminCreateRole, params);
export const updateRoleDescription = (role: string, description: string) =>
  request.put<ResponseData<AdminService.RoleDetail>>(
    adminUpdateRoleDescription(role),
    { description },
  );
export const deleteRole = (role: string) =>
  request.delete<ResponseData<ResponseData<never>>>(adminDeleteRole(role));
export const listRoles = () =>
  request.get<
    ResponseData<{ roles: AdminService.ListRoleItem[]; total: number }>
  >(adminListRoles);
export const listRolesWithPermission = () =>
  request.get<
    ResponseData<{
      roles: AdminService.ListRoleItemWithPermission[];
      total: number;
    }>
  >(adminListRolesWithPermission);
export const getRolePermissions = (role: string) =>
  request.get<ResponseData<AdminService.RoleDetailWithPermission>>(
    adminGetRolePermissions(role),
  );
export const assignRolePermissions = (
  role: string,
  params: AdminService.AssignRolePermissionInput,
) =>
  request.post<ResponseData<never>>(adminAssignRolePermissions(role), params);
export const revokeRolePermissions = (
  role: string,
  params: AdminService.RevokeRolePermissionInput,
) =>
  request.delete<ResponseData<never>>(adminRevokeRolePermissions(role), {
    data: params,
  });

export const updateUserRole = (username: string, role: string) =>
  request.put<ResponseData<never>>(adminUpdateUserRole(username), {
    role_name: role,
  });
export const getUserPermissions = (username: string) =>
  request.get<ResponseData<AdminService.UserDetailWithPermission>>(
    adminGetUserPermissions(username),
  );
export const listResources = () =>
  request.get<ResponseData<AdminService.ResourceType>>(adminListResources);

export default {
  login,
  logout,
  listUsers,
  createUser,
  showUserDetails: getUserDetails,
  updateUserStatus,
  updateUserPassword,
  deleteUser,
  listUserDatasets,
  listUserAgents,
};
