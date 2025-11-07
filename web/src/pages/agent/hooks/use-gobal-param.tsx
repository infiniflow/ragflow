import { useState } from 'react';

export const useGobalParam = () => {
  const [openGobalParamSettingModal, setOpenGobalParamSettingModal] =
    useState(false);
  return { openGobalParamSettingModal, setOpenGobalParamSettingModal };
};
