// src/components/ProfilePage.tsx
import { AvatarUpload } from '@/components/avatar-upload';
import PasswordInput from '@/components/originui/password-input';
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useTranslate } from '@/hooks/common-hooks';
import { TimezoneList } from '@/pages/user-setting/constants';
import { zodResolver } from '@hookform/resolvers/zod';
import { t } from 'i18next';
import { Loader2Icon, PenLine } from 'lucide-react';
import { FC, useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { EditType, modalTitle, useProfile } from './hooks/use-profile';

const baseSchema = z.object({
  userName: z
    .string()
    .min(1, { message: t('setting.usernameMessage') })
    .trim(),
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
const ProfilePage: FC = () => {
  const { t } = useTranslate('setting');

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

  const form = useForm<z.infer<typeof baseSchema | typeof passwordSchema>>({
    resolver: zodResolver(
      editType === EditType.editPassword ? passwordSchema : nameSchema,
    ),
    defaultValues: {
      userName: '',
      timeZone: '',
    },
    // shouldUnregister: true,
  });
  useEffect(() => {
    form.reset({ ...editForm, currPasswd: undefined });
  }, [editForm, form]);

  //   const ModalContent: FC = () => {
  //     // let content = null;
  //     // if (editType === EditType.editName) {
  //     //   content = editName();
  //     // }
  //     return (
  //       <>

  //       </>
  //     );
  //   };

  return (
    <div className="h-full bg-bg-base text-text-secondary p-5">
      {/* Header */}
      <header className="flex flex-col gap-1 justify-between items-start mb-6">
        <h1 className="text-2xl font-bold text-text-primary">{t('profile')}</h1>
        <div className="text-sm text-text-secondary mb-6">
          {t('profileDescription')}
        </div>
      </header>

      {/* Main Content */}
      <div className="max-w-3xl space-y-11 w-3/4">
        {/* Name */}
        <div className="flex items-start gap-4">
          <label className="w-[190px] text-sm font-medium">
            {t('username')}
          </label>
          <div className="flex-1 flex items-center gap-4 min-w-60">
            <div className="text-sm text-text-primary border border-border-button flex-1 rounded-md py-1.5 px-2">
              {profile.userName}
            </div>
            <Button
              variant={'secondary'}
              type="button"
              onClick={() => handleEditClick(EditType.editName)}
              className="text-sm text-text-secondary flex gap-1 px-1"
            >
              <PenLine size={12} /> Edit
            </Button>
          </div>
        </div>

        {/* Avatar */}
        <div className="flex items-start gap-4">
          <label className="w-[190px] text-sm font-medium">{t('avatar')}</label>
          <div className="flex items-center gap-4">
            <AvatarUpload
              value={profile.avatar}
              onChange={handleAvatarUpload}
              tips={'This will be displayed on your profile.'}
            />
          </div>
        </div>

        {/* Time Zone */}
        <div className="flex items-start gap-4">
          <label className="w-[190px] text-sm font-medium">
            {t('timezone')}
          </label>
          <div className="flex-1 flex items-center gap-4">
            <div className="text-sm text-text-primary border border-border-button flex-1 rounded-md py-1.5 px-2">
              {profile.timeZone}
            </div>
            <Button
              variant={'secondary'}
              type="button"
              onClick={() => handleEditClick(EditType.editTimeZone)}
              className="text-sm text-text-secondary flex gap-1 px-1"
            >
              <PenLine size={12} /> Edit
            </Button>
          </div>
        </div>

        {/* Email Address */}
        <div className="flex items-start gap-4">
          <label className="w-[190px] text-sm font-medium"> {t('email')}</label>
          <div className="flex-1 flex flex-col items-start gap-2">
            <div className="text-sm text-text-primary flex-1 rounded-md py-1.5 ">
              {profile.email}
            </div>
            <span className="text-text-secondary text-xs">
              {t('emailDescription')}
            </span>
          </div>
        </div>

        {/* Password */}
        <div className="flex items-start gap-4">
          <label className="w-[190px] text-sm font-medium">
            {t('password')}
          </label>
          <div className="flex-1 flex items-center gap-4">
            <div className="text-sm text-text-primary border border-border-button flex-1 rounded-md py-1.5 px-2">
              {profile.currPasswd ? '********' : ''}
            </div>
            <Button
              variant={'secondary'}
              type="button"
              onClick={() => handleEditClick(EditType.editPassword)}
              className="text-sm text-text-secondary flex gap-1 px-1"
            >
              <PenLine size={12} /> Edit
            </Button>
          </div>
        </div>
      </div>

      {editType && (
        <Modal
          title={modalTitle[editType]}
          open={isEditing}
          showfooter={false}
          onOpenChange={(open) => {
            if (!open) {
              handleCancel();
            }
          }}
          className="!w-[480px]"
        >
          {/* <ModalContent /> */}
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
                            {...field}
                            className="bg-bg-input border-border-default"
                          />
                        </FormControl>
                      </div>
                      <div className="flex w-full pt-1">
                        <div className="w-1/4"></div>
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
                        <Select
                          onValueChange={field.onChange}
                          value={field.value}
                        >
                          <FormControl className="w-full bg-bg-input border-border-default">
                            <SelectTrigger>
                              <SelectValue placeholder="Select a timeZone" />
                            </SelectTrigger>
                          </FormControl>
                          <SelectContent>
                            {TimezoneList.map((timeStr) => (
                              <SelectItem key={timeStr} value={timeStr}>
                                {timeStr}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
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
                            className="text-sm flex justify-between text-text-secondary whitespace-nowrap"
                          >
                            {t('currentPassword')}
                          </FormLabel>
                          <FormControl className="w-full">
                            <PasswordInput
                              {...field}
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
                <Button type="reset" variant="secondary">
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
    </div>
  );
};

export default ProfilePage;
