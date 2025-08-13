import PasswordInput from '@/components/originui/password-input';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useTranslate } from '@/hooks/common-hooks';
import { useFetchUserInfo, useSaveSetting } from '@/hooks/user-setting-hooks';
import { TimezoneList } from '@/pages/user-setting/constants';
import { rsaPsw } from '@/utils';
import { transformFile2Base64 } from '@/utils/file-util';
import { zodResolver } from '@hookform/resolvers/zod';
import { TFunction } from 'i18next';
import { Loader2Icon, Pencil, Upload } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
function defineSchema(
  t: TFunction<'translation', string>,
  showPasswordForm = false,
) {
  const baseSchema = z.object({
    userName: z
      .string()
      .min(1, { message: t('usernameMessage') })
      .trim(),
    avatarUrl: z.string().trim(),
    timeZone: z
      .string()
      .trim()
      .min(1, { message: t('timezonePlaceholder') }),
    email: z
      .string({ required_error: 'Please select an email to display.' })
      .trim()
      .regex(/^[A-Za-z0-9\u4e00-\u9fa5]+@[a-zA-Z0-9_-]+(\.[a-zA-Z0-9_-]+)+$/, {
        message: 'Enter a valid email address.',
      }),
  });

  if (showPasswordForm) {
    return baseSchema
      .extend({
        currPasswd: z
          .string({
            required_error: t('currentPasswordMessage'),
          })
          .trim()
          .min(1, { message: t('currentPasswordMessage') }),
        newPasswd: z
          .string({
            required_error: t('confirmPasswordMessage'),
          })
          .trim()
          .min(8, { message: t('confirmPasswordMessage') }),
        confirmPasswd: z
          .string({
            required_error: t('newPasswordDescription'),
          })
          .trim()
          .min(8, { message: t('newPasswordDescription') }),
      })
      .refine((data) => data.newPasswd === data.confirmPasswd, {
        message: t('confirmPasswordNonMatchMessage'),
        path: ['confirmPasswd'],
      });
  }

  return baseSchema;
}
export default function Profile() {
  const [avatarFile, setAvatarFile] = useState<File | null>(null);
  const [avatarBase64Str, setAvatarBase64Str] = useState(''); // Avatar Image base64
  const { data: userInfo } = useFetchUserInfo();
  const {
    saveSetting,
    loading: submitLoading,
    data: saveUserData,
  } = useSaveSetting();

  const { t } = useTranslate('setting');
  const [showPasswordForm, setShowPasswordForm] = useState(false);
  const FormSchema = defineSchema(t, showPasswordForm);
  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      userName: '',
      avatarUrl: '',
      timeZone: '',
      email: '',
      // currPasswd: '',
      // newPasswd: '',
      // confirmPasswd: '',
    },
    shouldUnregister: true,
  });

  useEffect(() => {
    // init user info when mounted
    form.setValue('email', userInfo?.email); // email
    form.setValue('userName', userInfo?.nickname); // nickname
    form.setValue('timeZone', userInfo?.timezone); // time zone
    // form.setValue('currPasswd', ''); // current password
    setAvatarBase64Str(userInfo?.avatar ?? '');
  }, [userInfo]);

  useEffect(() => {
    if (saveUserData === 0) {
      setShowPasswordForm(false);
      form.resetField('currPasswd');
      form.resetField('newPasswd');
      form.resetField('confirmPasswd');
    }
    console.log('saveUserData', saveUserData);
  }, [saveUserData]);

  useEffect(() => {
    if (avatarFile) {
      // make use of img compression transformFile2Base64
      (async () => {
        setAvatarBase64Str(await transformFile2Base64(avatarFile));
      })();
    }
  }, [avatarFile]);

  function onSubmit(data: z.infer<typeof FormSchema>) {
    const payload: Partial<{
      nickname: string;
      password: string;
      new_password: string;
      avatar: string;
      timezone: string;
    }> = {
      nickname: data.userName,
      avatar: avatarBase64Str,
      timezone: data.timeZone,
    };

    if (showPasswordForm && 'currPasswd' in data && 'newPasswd' in data) {
      payload.password = rsaPsw(data.currPasswd!) as string;
      payload.new_password = rsaPsw(data.newPasswd!) as string;
    }
    saveSetting(payload);
  }

  useEffect(() => {
    if (showPasswordForm) {
      form.register('currPasswd');
      form.register('newPasswd');
      form.register('confirmPasswd');
    } else {
      form.unregister(['currPasswd', 'newPasswd', 'confirmPasswd']);
    }
  }, [showPasswordForm]);
  return (
    <section className="p-8">
      <h1 className="text-3xl font-bold">{t('profile')}</h1>
      <div className="text-sm text-muted-foreground mb-6">
        {t('profileDescription')}
      </div>
      <div>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(onSubmit)}
            className="block space-y-6"
          >
            {/* Username Field */}
            <FormField
              control={form.control}
              name="userName"
              render={({ field }) => (
                <FormItem className=" items-center space-y-0 ">
                  <div className="flex w-[640px]">
                    <FormLabel className="text-sm text-muted-foreground whitespace-nowrap w-1/4">
                      <span className="text-red-600">*</span>
                      {t('username')}
                    </FormLabel>
                    <FormControl className="w-3/4">
                      <Input placeholder="" {...field} />
                    </FormControl>
                  </div>
                  <div className="flex w-[640px] pt-1">
                    <div className="w-1/4"></div>
                    <FormMessage />
                  </div>
                </FormItem>
              )}
            />

            {/* Avatar Field */}
            <FormField
              control={form.control}
              name="avatarUrl"
              render={({ field }) => (
                <FormItem className="flex items-center space-y-0">
                  <div className="flex w-[640px]">
                    <FormLabel className="text-sm text-muted-foreground whitespace-nowrap w-1/4">
                      Avatar
                    </FormLabel>
                    <FormControl className="w-3/4">
                      <div className="flex justify-start items-end space-x-2">
                        <div className="relative group">
                          {!avatarBase64Str ? (
                            <div className="w-[64px] h-[64px] grid place-content-center">
                              <div className="flex flex-col items-center">
                                <Upload />
                                <p>Upload</p>
                              </div>
                            </div>
                          ) : (
                            <div className="w-[64px] h-[64px] relative grid place-content-center">
                              <Avatar className="w-[64px] h-[64px] rounded-md">
                                <AvatarImage
                                  className="block"
                                  src={avatarBase64Str}
                                  alt=""
                                />
                                <AvatarFallback className="rounded-md"></AvatarFallback>
                              </Avatar>
                              <div className="absolute inset-0 bg-[#000]/20 group-hover:bg-[#000]/60">
                                <Pencil
                                  size={16}
                                  className="absolute right-1 bottom-1 opacity-50 hidden group-hover:block"
                                />
                              </div>
                            </div>
                          )}
                          <Input
                            placeholder=""
                            {...field}
                            type="file"
                            title=""
                            accept="image/*"
                            className="absolute top-0 left-0 w-full h-full opacity-0 cursor-pointer"
                            onChange={(ev) => {
                              const file = ev.target?.files?.[0];
                              if (
                                /\.(jpg|jpeg|png|webp|bmp)$/i.test(
                                  file?.name ?? '',
                                )
                              ) {
                                setAvatarFile(file!);
                              }
                              ev.target.value = '';
                            }}
                          />
                        </div>
                        <div className="margin-1 text-muted-foreground">
                          {t('avatarTip')}
                        </div>
                      </div>
                    </FormControl>
                  </div>
                  <div className="flex w-[640px] pt-1">
                    <div className="w-1/4"></div>
                    <FormMessage />
                  </div>
                </FormItem>
              )}
            />

            {/* Time Zone Field */}
            <FormField
              control={form.control}
              name="timeZone"
              render={({ field }) => (
                <FormItem className="items-center space-y-0">
                  <div className="flex w-[640px]">
                    <FormLabel className="text-sm text-muted-foreground whitespace-nowrap w-1/4">
                      <span className="text-red-600">*</span>
                      {t('timezone')}
                    </FormLabel>
                    <Select onValueChange={field.onChange} value={field.value}>
                      <FormControl className="w-3/4">
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
                  <div className="flex w-[640px] pt-1">
                    <div className="w-1/4"></div>
                    <FormMessage />
                  </div>
                </FormItem>
              )}
            />

            {/* Email Address Field */}
            <FormField
              control={form.control}
              name="email"
              render={({ field }) => (
                <div>
                  <FormItem className="items-center space-y-0">
                    <div className="flex w-[640px]">
                      <FormLabel className="text-sm text-muted-foreground whitespace-nowrap w-1/4">
                        {t('email')}
                      </FormLabel>
                      <FormControl className="w-3/4">
                        <>{field.value}</>
                      </FormControl>
                    </div>
                    <div className="flex w-[640px] pt-1">
                      <div className="w-1/4"></div>
                      <FormMessage />
                    </div>
                  </FormItem>
                  <div className="flex w-[640px] pt-1">
                    <p className="w-1/4">&nbsp;</p>
                    <p className="text-sm text-muted-foreground whitespace-nowrap w-3/4">
                      {t('emailDescription')}
                    </p>
                  </div>
                </div>
              )}
            />

            {/* Password Section */}
            <div className="pb-6">
              <div className="flex items-center justify-start">
                <h1 className="text-3xl font-bold">{t('password')}</h1>
                <Button
                  type="button"
                  className="bg-transparent hover:bg-transparent border text-muted-foreground hover:text-white ml-10"
                  onClick={() => {
                    setShowPasswordForm(!showPasswordForm);
                  }}
                >
                  {t('changePassword')}
                </Button>
              </div>
              <div className="text-sm text-muted-foreground">
                {t('passwordDescription')}
              </div>
            </div>
            {/* Password Form */}
            {showPasswordForm && (
              <>
                <FormField
                  control={form.control}
                  name="currPasswd"
                  render={({ field }) => (
                    <FormItem className="items-center space-y-0">
                      <div className="flex w-[640px]">
                        <FormLabel className="text-sm text-muted-foreground whitespace-nowrap w-2/5">
                          <span className="text-red-600">*</span>
                          {t('currentPassword')}
                        </FormLabel>
                        <FormControl className="w-3/5">
                          <PasswordInput {...field} />
                        </FormControl>
                      </div>
                      <div className="flex w-[640px] pt-1">
                        <div className="min-w-[170px] max-w-[170px]"></div>
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
                      <div className="flex w-[640px]">
                        <FormLabel className="text-sm text-muted-foreground whitespace-nowrap w-2/5">
                          <span className="text-red-600">*</span>
                          {t('newPassword')}
                        </FormLabel>
                        <FormControl className="w-3/5">
                          <PasswordInput {...field} />
                        </FormControl>
                      </div>
                      <div className="flex w-[640px] pt-1">
                        <div className="min-w-[170px] max-w-[170px]"></div>
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
                      <div className="flex w-[640px]">
                        <FormLabel className="text-sm text-muted-foreground whitespace-nowrap w-2/5">
                          <span className="text-red-600">*</span>
                          {t('confirmPassword')}
                        </FormLabel>
                        <FormControl className="w-3/5">
                          <PasswordInput
                            {...field}
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
                      <div className="flex w-[640px] pt-1">
                        <div className="min-w-[170px] max-w-[170px]">
                          &nbsp;
                        </div>
                        <FormMessage />
                      </div>
                    </FormItem>
                  )}
                />
              </>
            )}
            <div className="w-[640px] text-right space-x-4">
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
      </div>
    </section>
  );
}
