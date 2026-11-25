/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

// src/components/ProfilePage.tsx
import { AvatarUpload } from '@/components/avatar-upload';
import PasswordInput from '@/components/originui/password-input';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { TruncatedText } from '@/components/truncated-text';
import { Button } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Modal } from '@/components/ui/modal/modal';
import { useChangeLanguage } from '@/hooks/logic-hooks';
import { useFetchUserInfo } from '@/hooks/use-user-setting-request';
import { DEFAULT_LANGUAGE_CODE, supportedLanguages } from '@/locales/config';
import { TimezoneList } from '@/pages/user-setting/constants';
import { zodResolver } from '@hookform/resolvers/zod';
import { t } from 'i18next';
import { Loader2Icon, PenLine } from 'lucide-react';
import {
  type FC,
  type PropsWithChildren,
  useCallback,
  useEffect,
  useMemo,
} from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { NICKNAME_MAX_LENGTH, NICKNAME_PATTERN } from './constants';
import { EditType, modalTitle, useProfile } from './hooks/use-profile';

const timezoneOptions = TimezoneList.map(({ name }) => ({
  value: name,
  label: name,
}));

const languageOptions = supportedLanguages.map(({ code, displayName }) => ({
  value: code,
  label: displayName,
}));

const baseSchema = z.object({
  userName: z
    .string()
    .trim()
    .min(1, { message: t('setting.usernameMessage') })
    .max(NICKNAME_MAX_LENGTH, {
      message: t('setting.usernameMaxLength', { max: NICKNAME_MAX_LENGTH }),
    })
    .regex(NICKNAME_PATTERN, {
      message: t('setting.usernameInvalidCharacters'),
    }),
  timeZone: z
    .string()
    .trim()
    .min(1, { message: t('setting.timezonePlaceholder') }),
});

const nameSchema = baseSchema.extend({
  currPasswd: z.string().optional(),
  newPasswd: z.string().optional(),
  confirmPasswd: z.string().optional(),
});

const passwordSchema = baseSchema
  .extend({
    currPasswd: z
      .string({
        required_error: t('setting.currentPasswordMessage'),
      })
      .trim(),
    newPasswd: z
      .string({
        required_error: t('setting.newPasswordMessage'),
      })
      .trim()
      .min(8, { message: t('setting.newPasswordDescription') }),
    confirmPasswd: z
      .string({
        required_error: t('setting.confirmPasswordMessage'),
      })
      .trim()
      .min(8, { message: t('setting.newPasswordDescription') }),
  })
  .superRefine((data, ctx) => {
    if (
      data.newPasswd &&
      data.confirmPasswd &&
      data.newPasswd !== data.confirmPasswd
    ) {
      ctx.addIssue({
        path: ['confirmPasswd'],
        message: t('setting.confirmPasswordNonMatchMessage'),
        code: z.ZodIssueCode.custom,
      });
    }
  });

type ProfileFieldProps = PropsWithChildren<{
  label: string;
  controlId?: string;
}>;

function ProfileField({ label, controlId, children }: ProfileFieldProps) {
  const LabelTag = controlId ? 'label' : 'span';

  return (
    <div className="grid grid-cols-[7rem_minmax(0,1fr)] items-start gap-4 sm:grid-cols-[9rem_minmax(0,1fr)] lg:grid-cols-[12rem_minmax(0,1fr)]">
      <LabelTag
        htmlFor={controlId}
        className="pt-1.5 text-sm font-medium text-text-secondary"
      >
        {label}
      </LabelTag>
      <div className="min-w-0">{children}</div>
    </div>
  );
}

type ProfileValueProps = {
  value: string;
  editLabel: string;
  onEdit: () => void;
};

function ProfileValue({ value, editLabel, onEdit }: ProfileValueProps) {
  return (
    <div className="flex h-8 min-w-0 items-center gap-1 rounded-md bg-bg-input px-2">
      <TruncatedText
        as="span"
        className="min-w-0 flex-1 truncate text-sm text-text-primary"
        tooltip={value}
      >
        {value}
      </TruncatedText>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label={editLabel}
        title={editLabel}
        onClick={onEdit}
        className="shrink-0"
      >
        <PenLine className="size-4" />
      </Button>
    </div>
  );
}

const ProfilePage: FC = () => {
  const { t } = useTranslation('translation', { keyPrefix: 'setting' });

  const {
    profile,
    editType,
    isEditing,
    submitLoading,
    editForm,
    handleEditClick,
    handleCancel,
    handleSave,
    handleAvatarUpload,
  } = useProfile();
  const changeLanguage = useChangeLanguage();
  const {
    data: { language = DEFAULT_LANGUAGE_CODE },
  } = useFetchUserInfo();

  const handleNameEdit = useCallback(() => {
    handleEditClick(EditType.editName);
  }, [handleEditClick]);

  const handleTimeZoneEdit = useCallback(() => {
    handleEditClick(EditType.editTimeZone);
  }, [handleEditClick]);

  const handlePasswordEdit = useCallback(() => {
    handleEditClick(EditType.editPassword);
  }, [handleEditClick]);

  const form = useForm<z.infer<typeof baseSchema | typeof passwordSchema>>({
    resolver: zodResolver(
      editType === EditType.editPassword ? passwordSchema : nameSchema,
    ),
    defaultValues: {
      userName: '',
      timeZone: '',
    },
  });
  useEffect(() => {
    form.reset({ ...editForm, currPasswd: undefined });
  }, [editForm, form]);

  const timezone = useMemo(() => {
    const tz = TimezoneList.find((tz) => tz.name === profile.timeZone);
    return tz?.name ?? '';
  }, [profile.timeZone]);

  return (
    <article className="h-full w-full min-w-0 overflow-y-auto">
      <h2 className="text-2xl font-medium text-text-primary">{t('profile')}</h2>

      <div className="mt-12 space-y-11">
        <ProfileField label={t('username')}>
          <ProfileValue
            value={profile.userName}
            editLabel={t('editName')}
            onEdit={handleNameEdit}
          />
        </ProfileField>

        <ProfileField label={t('avatar')}>
          <AvatarUpload
            value={profile.avatar}
            onChange={handleAvatarUpload}
            tips={t('avatarTip')}
          />
        </ProfileField>

        <ProfileField label={t('timezone')}>
          <ProfileValue
            value={timezone}
            editLabel={t('editTimeZone')}
            onEdit={handleTimeZoneEdit}
          />
        </ProfileField>

        <ProfileField label={t('email')}>
          <div className="min-w-0 py-1.5">
            <TruncatedText
              className="truncate text-sm text-text-primary"
              tooltip={profile.email}
            >
              {profile.email}
            </TruncatedText>
            <p className="mt-2 break-words text-xs text-text-secondary">
              {t('emailDescription')}
            </p>
          </div>
        </ProfileField>

        {(!profile.loginChannel || profile.loginChannel === 'password') && (
          <ProfileField label={t('password')}>
            <ProfileValue
              value="********"
              editLabel={t('editPassword')}
              onEdit={handlePasswordEdit}
            />
          </ProfileField>
        )}

        <ProfileField
          label={t('displayLanguage')}
          controlId="profile-display-language"
        >
          <SelectWithSearch
            options={languageOptions}
            value={language}
            onChange={changeLanguage}
            triggerId="profile-display-language"
            triggerClassName="w-full border-0 bg-bg-input"
          />
        </ProfileField>
      </div>

      {editType && (
        <Modal
          title={t(modalTitle[editType])}
          open={isEditing}
          showfooter={false}
          maskClosable={false}
          titleClassName="text-base"
          onOpenChange={(open) => {
            if (!open) {
              handleCancel();
            }
          }}
          className="!w-[480px]"
        >
          <Form {...form}>
            <form
              onSubmit={form.handleSubmit((data) => handleSave(data as any))}
              className="flex flex-col mt-6 mb-8 ml-2 space-y-6 "
            >
              {editType === EditType.editName && (
                <FormField
                  control={form.control}
                  name="userName"
                  render={({ field }) => (
                    <FormItem className=" items-center space-y-0 ">
                      <div className="flex flex-col w-full gap-2">
                        <FormLabel className="text-sm text-text-secondary whitespace-nowrap">
                          {t('username')}
                        </FormLabel>
                        <FormControl className="w-full">
                          <Input
                            placeholder=""
                            maxLength={NICKNAME_MAX_LENGTH}
                            {...field}
                            className="bg-bg-input border-border-default"
                          />
                        </FormControl>
                      </div>
                      <div className="flex w-full pt-1">
                        <FormMessage />
                      </div>
                    </FormItem>
                  )}
                />
              )}

              {editType === EditType.editTimeZone && (
                <FormField
                  control={form.control}
                  name="timeZone"
                  render={({ field }) => (
                    <FormItem className="items-center space-y-0">
                      <div className="flex flex-col w-full gap-2">
                        <FormLabel className="text-sm text-text-secondary whitespace-nowrap">
                          {t('timezone')}
                        </FormLabel>
                        <SelectWithSearch
                          options={timezoneOptions}
                          placeholder="Select a timeZone"
                          onChange={field.onChange}
                          value={field.value}
                        />
                      </div>
                      <div className="flex w-full pt-1">
                        <div className="w-1/4"></div>
                        <FormMessage />
                      </div>
                    </FormItem>
                  )}
                />
              )}

              {editType === EditType.editPassword && (
                <>
                  <FormField
                    control={form.control}
                    name="currPasswd"
                    render={({ field }) => (
                      <FormItem className="items-center space-y-0">
                        <div className="flex flex-col w-full gap-2">
                          <FormLabel
                            required
                            className="text-sm flex text-text-secondary whitespace-nowrap"
                          >
                            {t('currentPassword')}
                          </FormLabel>
                          <FormControl className="w-full">
                            <PasswordInput
                              {...field}
                              placeholder={t('currentPassword')}
                              autoComplete="current-password"
                              className="bg-bg-input border-border-default"
                            />
                          </FormControl>
                        </div>
                        <div className="flex w-full pt-1">
                          <FormMessage />
                        </div>
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name="newPasswd"
                    render={({ field }) => (
                      <FormItem className=" items-center space-y-0">
                        <div className="flex flex-col w-full gap-2">
                          <FormLabel
                            required
                            className="text-sm text-text-secondary whitespace-nowrap"
                          >
                            {t('newPassword')}
                          </FormLabel>
                          <FormControl className="w-full">
                            <PasswordInput
                              {...field}
                              placeholder={t('newPassword')}
                              autoComplete="new-password"
                              className="bg-bg-input border-border-default"
                            />
                          </FormControl>
                        </div>
                        <div className="flex w-full pt-1">
                          <FormMessage />
                        </div>
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name="confirmPasswd"
                    render={({ field }) => (
                      <FormItem className=" items-center space-y-0">
                        <div className="flex flex-col w-full gap-2">
                          <FormLabel
                            required
                            className="text-sm text-text-secondary whitespace-nowrap"
                          >
                            {t('confirmPassword')}
                          </FormLabel>
                          <FormControl className="w-full">
                            <PasswordInput
                              {...field}
                              placeholder={t('confirmPassword')}
                              className="bg-bg-input border-border-default"
                              autoComplete="new-password"
                              onBlur={() => {
                                form.trigger('confirmPasswd');
                              }}
                              onChange={(ev) => {
                                form.setValue(
                                  'confirmPasswd',
                                  ev.target.value.trim(),
                                );
                              }}
                            />
                          </FormControl>
                        </div>
                        <div className="flex w-full pt-1">
                          <FormMessage />
                        </div>
                      </FormItem>
                    )}
                  />
                </>
              )}

              <div className="w-full text-right space-x-4 !mt-11">
                <Button type="reset" variant="secondary" onClick={handleCancel}>
                  {t('cancel')}
                </Button>
                <Button type="submit" disabled={submitLoading}>
                  {submitLoading && <Loader2Icon className="animate-spin" />}
                  {t('save', { keyPrefix: 'common' })}
                </Button>
              </div>
            </form>
          </Form>
        </Modal>
      )}
    </article>
  );
};

export default ProfilePage;
