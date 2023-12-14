import axios from 'axios';
import type { RouteRecordNormalized } from 'vue-router';
import { UserState } from '@/store/modules/user/types';

export interface LoginData {
  username: string
  password: string
}

export interface LoginRes {
  token: string
}
export function login(data: LoginData) {
  return useAxiosApi<LoginRes>('/api/user/login', {
    method: 'POST',
    data,
  });
}

export function logout() {
  return useAxiosApi<LoginRes>('/api/user/logout', {
    method: 'POST',
  });
}

export function getUserInfo() {
  return useAxiosApi<UserState>('/api/user/info', {
    method: 'POST',
  });
}

export function getMenuList() {
  return useAxiosApi<RouteRecordNormalized[]>('/api/user/menu', {
    method: 'POST',
  });
}
