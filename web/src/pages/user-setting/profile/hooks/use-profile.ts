// src/hooks/useProfile.ts
import { DEFAULT_TIMEZONE } from '@/constants/setting';
import {
  useFetchUserInfo,
  useSaveSetting,
} from '@/hooks/use-user-setting-request';
import { TimezoneList } from '@/pages/user-setting/constants';
import { rsaPsw } from '@/utils';
import { useCallback, useEffect, useState } from 'react';

interface ProfileData {
  userName: string;
  timeZone: string;
  currPasswd?: string;
  newPasswd?: string;
  avatar: string;
  email: string;
  confirmPasswd?: string;
}

export const EditType = {
  editName: 'editName',
  editTimeZone: 'editTimeZone',
  editPassword: 'editPassword',
} as const;

export type IEditType = keyof typeof EditType;

export const modalTitle = {
  [EditType.editName]: 'Edit Name',
  [EditType.editTimeZone]: 'Edit Time Zone',
  [EditType.editPassword]: 'Edit Password',
} as const;

const normalizeTimezone = (tz: string | undefined): string => {
  if (!tz) return '';
  // Support both backend format "UTC+8\tAsia/Shanghai" and frontend format "GMT+08:00 Asia/Shanghai"
  const parts = tz.split(/\t|\s+/);
  const ianaName = parts.length > 1 ? parts[parts.length - 1] : tz;
  return TimezoneList.find((item) => item.id === ianaName)?.name ?? '';
};

export const useProfile = () => {
  const { data: userInfo } = useFetchUserInfo();
  const [profile, setProfile] = useState<ProfileData>({
    userName: '',
    avatar: '',
    timeZone: '',
    email: '',
    currPasswd: '',
  });

  const [editType, setEditType] = useState<IEditType>(EditType.editName);
  const [isEditing, setIsEditing] = useState(false);
  const [editForm, setEditForm] = useState<Partial<ProfileData>>({});
  const {
    saveSetting,
    loading: submitLoading,
    data: saveSettingData,
  } = useSaveSetting();

  useEffect(() => {
    const profile = {
      userName: userInfo.nickname,
      timeZone: normalizeTimezone(userInfo.timezone) || DEFAULT_TIMEZONE?.name,
      avatar: userInfo.avatar || '',
      email: userInfo.email,
      currPasswd: userInfo.password,
    };
    setProfile(profile);
  }, [userInfo, setProfile]);

  useEffect(() => {
    if (saveSettingData === 0) {
      setIsEditing(false);
      setEditForm({});
    }
  }, [saveSettingData]);
  const onSubmit = (newProfile: ProfileData) => {
    const payload: Partial<{
      nickname: string;
      password: string;
      new_password: string;
      avatar: string;
      timezone: string;
    }> = {
      nickname: newProfile.userName,
      avatar: newProfile.avatar,
      timezone: newProfile.timeZone,
    };

    if (
      'currPasswd' in newProfile &&
      'newPasswd' in newProfile &&
      newProfile.currPasswd &&
      newProfile.newPasswd
    ) {
      payload.password = rsaPsw(newProfile.currPasswd!) as string;
      payload.new_password = rsaPsw(newProfile.newPasswd!) as string;
    }
    if (editType === EditType.editName && payload.nickname) {
      saveSetting({ nickname: payload.nickname });
      setProfile(newProfile);
    }
    if (editType === EditType.editTimeZone && payload.timezone) {
      saveSetting({ timezone: payload.timezone });
      setProfile(newProfile);
    }
    if (editType === EditType.editPassword && payload.password) {
      saveSetting({
        password: payload.password,
        new_password: payload.new_password,
      });
      setProfile(newProfile);
    }
  };

  const handleEditClick = useCallback(
    (type: IEditType) => {
      setEditForm(profile);
      setEditType(type);
      setIsEditing(true);
    },
    [profile],
  );

  const handleCancel = useCallback(() => {
    setIsEditing(false);
    setEditForm({});
  }, []);

  const handleSave = (data: ProfileData) => {
    const newProfile = { ...profile, ...data };

    onSubmit(newProfile);
  };

  const handleAvatarUpload = (avatar: string) => {
    setProfile((prev) => ({ ...prev, avatar }));
    saveSetting({ avatar });
  };

  return {
    profile,
    setProfile,
    submitLoading: submitLoading,
    isEditing,
    editType,
    editForm,
    handleEditClick,
    handleCancel,
    handleSave,
    handleAvatarUpload,
  };
};
