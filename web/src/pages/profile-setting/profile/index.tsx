import PasswordInput from '@/components/password-input';
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

function defineSchema(t: TFunction<'translation', string>) {
  return z
    .object({
      userName: z
        .string()
        .min(1, {
          message: t('usernameMessage'),
        })
        .trim(),
      avatarUrl: z.string().trim(),
      timeZone: z
        .string()
        .trim()
        .min(1, {
          message: t('timezonePlaceholder'),
        }),
      email: z
        .string({
          required_error: 'Please select an email to display.',
        })
        .trim()
        .regex(
          /^[A-Za-z0-9\u4e00-\u9fa5]+@[a-zA-Z0-9_-]+(\.[a-zA-Z0-9_-]+)+$/,
          {
            message: 'Enter a valid email address.',
          },
        ),
      currPasswd: z
        .string()
        .trim()
        .min(1, {
          message: t('currentPasswordMessage'),
        }),
      newPasswd: z
        .string()
        .trim()
        .min(8, {
          message: t('confirmPasswordMessage'),
        }),
      confirmPasswd: z
        .string()
        .trim()
        .min(8, {
          message: t('newPasswordDescription'),
        }),
    })
    .refine((data) => data.newPasswd === data.confirmPasswd, {
      message: t('confirmPasswordNonMatchMessage'),
      path: ['confirmPasswd'],
    });
}

export default function Profile() {
  const [avatarFile, setAvatarFile] = useState<File | null>(null);
  const [avatarBase64Str, setAvatarBase64Str] = useState(''); // Avatar Image base64
  const { data: userInfo } = useFetchUserInfo();
  const { saveSetting, loading: submitLoading } = useSaveSetting();

  const { t } = useTranslate('setting');
  const FormSchema = defineSchema(t);

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      userName: '',
      avatarUrl: '',
      timeZone: '',
      email: '',
      currPasswd: '',
      newPasswd: '',
      confirmPasswd: '',
    },
  });

  useEffect(() => {
    // init user info when mounted
    form.setValue('email', userInfo?.email); // email
    form.setValue('userName', userInfo?.nickname); // nickname
    form.setValue('timeZone', userInfo?.timezone); // time zone
    form.setValue('currPasswd', ''); // current password
    setAvatarBase64Str(userInfo?.avatar ?? '');
  }, [userInfo]);

  useEffect(() => {
    if (avatarFile) {
      // make use of img compression transformFile2Base64
      (async () => {
        setAvatarBase64Str(await transformFile2Base64(avatarFile));
      })();
    }
  }, [avatarFile]);

  function onSubmit(data: z.infer<typeof FormSchema>) {
    // toast('You submitted the following values', {
    //   description: (
    //     <pre className="mt-2 w-[320px] rounded-md bg-neutral-950 p-4">
    //       <code className="text-white">{JSON.stringify(data, null, 2)}</code>
    //     </pre>
    //   ),
    // });
    // console.log('data=', data);
    // final submit form
    saveSetting({
      nickname: data.userName,
      password: rsaPsw(data.currPasswd) as string,
      new_password: rsaPsw(data.newPasswd) as string,
      avatar: avatarBase64Str,
      timezone: data.timeZone,
    });
  }

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
            <FormField
              control={form.control}
              name="userName"
              render={({ field }) => (
                <FormItem className=" items-center space-y-0 ">
                  <div className="flex w-[600px]">
                    <FormLabel className="text-sm text-muted-foreground whitespace-nowrap w-1/4">
                      <span className="text-red-600">*</span>
                      {t('username')}
                    </FormLabel>
                    <FormControl className="w-3/4">
                      <Input
                        placeholder=""
                        {...field}
                        className="bg-colors-background-inverse-weak"
                      />
                    </FormControl>
                  </div>
                  <div className="flex w-[600px] pt-1">
                    <div className="w-1/4"></div>
                    <FormMessage />
                  </div>
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="avatarUrl"
              render={({ field }) => (
                <FormItem className="flex items-center space-y-0">
                  <div className="flex w-[600px]">
                    <FormLabel className="text-sm text-muted-foreground whitespace-nowrap w-1/4">
                      Avatar
                    </FormLabel>
                    <FormControl className="w-3/4">
                      <>
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
                              <Avatar className="w-[64px] h-[64px]">
                                <AvatarImage
                                  className=" block"
                                  src={avatarBase64Str}
                                  alt=""
                                />
                                <AvatarFallback></AvatarFallback>
                              </Avatar>
                              <div className="absolute inset-0 bg-[#000]/20 group-hover:bg-[#000]/60">
                                <Pencil
                                  size={20}
                                  className="absolute right-2 bottom-0 opacity-50 hidden group-hover:block"
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
                      </>
                    </FormControl>
                  </div>
                  <div className="flex w-[600px] pt-1">
                    <div className="w-1/4"></div>
                    <FormMessage />
                  </div>
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="timeZone"
              render={({ field }) => (
                <FormItem className="items-center space-y-0">
                  <div className="flex w-[600px]">
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
                  <div className="flex w-[600px] pt-1">
                    <div className="w-1/4"></div>
                    <FormMessage />
                  </div>
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="email"
              render={({ field }) => (
                <div>
                  <FormItem className="items-center space-y-0">
                    <div className="flex w-[600px]">
                      <FormLabel className="text-sm text-muted-foreground whitespace-nowrap w-1/4">
                        {t('email')}
                      </FormLabel>
                      <FormControl className="w-3/4">
                        <Input
                          placeholder="Alex@gmail.com"
                          disabled
                          {...field}
                        />
                      </FormControl>
                    </div>
                    <div className="flex w-[600px] pt-1">
                      <div className="w-1/4"></div>
                      <FormMessage />
                    </div>
                  </FormItem>
                  <div className="flex w-[600px] pt-1">
                    <p className="w-1/4">&nbsp;</p>
                    <p className="text-sm text-muted-foreground whitespace-nowrap w-3/4">
                      {t('emailDescription')}
                    </p>
                  </div>
                </div>
              )}
            />
            <div className="h-[10px]"></div>
            <div className="pb-6">
              <h1 className="text-3xl font-bold">{t('password')}</h1>
              <div className="text-sm text-muted-foreground">
                {t('passwordDescription')}
              </div>
            </div>
            <div className="h-0 overflow-hidden absolute">
              <input type="password" className=" w-0 height-0 opacity-0" />
            </div>
            <FormField
              control={form.control}
              name="currPasswd"
              render={({ field }) => (
                <FormItem className=" items-center space-y-0">
                  <div className="flex w-[600px]">
                    <FormLabel className="text-sm text-muted-foreground whitespace-nowrap w-2/5">
                      <span className="text-red-600">*</span>
                      {t('currentPassword')}
                    </FormLabel>
                    <FormControl className="w-3/5">
                      <PasswordInput {...field} />
                    </FormControl>
                  </div>
                  <div className="flex w-[600px] pt-1">
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
                  <div className="flex w-[600px]">
                    <FormLabel className="text-sm text-muted-foreground whitespace-nowrap w-2/5">
                      <span className="text-red-600">*</span>
                      {t('newPassword')}
                    </FormLabel>
                    <FormControl className="w-3/5">
                      <PasswordInput {...field} />
                    </FormControl>
                  </div>
                  <div className="flex w-[600px] pt-1">
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
                  <div className="flex w-[600px]">
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
                  <div className="flex w-[600px] pt-1">
                    <div className="min-w-[170px] max-w-[170px]">&nbsp;</div>
                    <FormMessage />
                  </div>
                </FormItem>
              )}
            />
            <div className="w-[600px] text-right space-x-4">
              <Button variant="secondary">{t('cancel')}</Button>
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
