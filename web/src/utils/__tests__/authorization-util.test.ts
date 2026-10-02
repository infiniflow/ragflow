import {
  AdminAuthorization,
  AdminToken,
  AdminUserInfo,
  Authorization,
  Token,
  UserInfo,
} from '@/constants/authorization';
import storage, {
  adminStorage,
  getAdminAuthorization,
  getAuthorization,
} from '../authorization-util';

describe('authorization-util session isolation', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('stores user and admin sessions under different localStorage keys', () => {
    storage.setItems({
      Authorization: 'Bearer user-token',
      Token: 'user-access',
      userInfo: JSON.stringify({ email: 'user@example.com' }),
    } as Record<string, string>);
    adminStorage.setItems({
      Authorization: 'Bearer admin-token',
      Token: 'admin-access',
      userInfo: JSON.stringify({ email: 'admin@example.com' }),
    } as Record<string, string>);

    expect(localStorage.getItem(Authorization)).toBe('Bearer user-token');
    expect(localStorage.getItem(Token)).toBe('user-access');
    expect(localStorage.getItem(AdminAuthorization)).toBe('Bearer admin-token');
    expect(localStorage.getItem(AdminToken)).toBe('admin-access');
    expect(getAuthorization()).toBe('Bearer user-token');
    expect(getAdminAuthorization()).toBe('Bearer admin-token');
  });

  it('does not clear user session when admin session is removed', () => {
    storage.setAuthorization('Bearer user-token');
    adminStorage.setAuthorization('Bearer admin-token');

    adminStorage.removeAll();

    expect(storage.getAuthorization()).toBe('Bearer user-token');
    expect(adminStorage.getAuthorization()).toBeNull();
    expect(localStorage.getItem(AdminAuthorization)).toBeNull();
  });

  it('does not clear admin session when user session is removed', () => {
    storage.setAuthorization('Bearer user-token');
    adminStorage.setAuthorization('Bearer admin-token');

    storage.removeAll();

    expect(adminStorage.getAuthorization()).toBe('Bearer admin-token');
    expect(storage.getAuthorization()).toBeNull();
    expect(localStorage.getItem(UserInfo)).toBeNull();
    expect(localStorage.getItem(AdminUserInfo)).toBeNull();
  });

  it('overwriting admin login does not overwrite user login keys', () => {
    storage.setItems({
      Authorization: 'Bearer user-token',
      Token: 'user-access',
      userInfo: JSON.stringify({ name: 'Regular User' }),
    } as Record<string, string>);

    adminStorage.setItems({
      Authorization: 'Bearer admin-token',
      Token: 'admin-access',
      userInfo: JSON.stringify({ name: 'Admin User' }),
    } as Record<string, string>);

    expect(localStorage.getItem(Authorization)).toBe('Bearer user-token');
    expect(localStorage.getItem(Token)).toBe('user-access');
    expect(JSON.parse(localStorage.getItem(UserInfo)!)).toEqual({
      name: 'Regular User',
    });
  });
});
