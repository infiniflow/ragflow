export const PROFILE_SETTINGS_PATH = '/user-setting/profile';
export const MODEL_SETTINGS_PATH = '/user-setting/model';

const normalizePath = (path: string) => {
  const normalized = path.replace(/\/+$/, '');
  return normalized || '/';
};

export const canAccessUserSettingsPath = (
  path: string,
  isSuperuser: boolean,
  adminOnly = false,
) => {
  if (isSuperuser) return true;
  if (adminOnly) return false;

  const normalizedPath = normalizePath(path);
  return [PROFILE_SETTINGS_PATH, MODEL_SETTINGS_PATH].includes(normalizedPath);
};
