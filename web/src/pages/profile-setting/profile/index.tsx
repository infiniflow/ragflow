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
import { useFetchUserInfo } from '@/hooks/user-setting-hooks';
import { TimezoneList } from '@/pages/user-setting/constants';
import { zodResolver } from '@hookform/resolvers/zod';
import { Pencil, Upload } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

const FormSchema = z
  .object({
    userName: z
      .string()
      .min(1, {
        message: 'user name is required.',
      })
      .trim(),
    avatarUrl: z.string().trim(),
    timeZone: z.string().trim().min(1, {
      message: 'Enter a time zone',
    }),
    email: z
      .string({
        required_error: 'Please select an email to display.',
      })
      .trim()
      .regex(/^[A-Za-z0-9\u4e00-\u9fa5]+@[a-zA-Z0-9_-]+(\.[a-zA-Z0-9_-]+)+$/, {
        message: 'Enter a valid email address.',
      }),
    currPasswd: z.string().trim().min(1, {
      message: 'Current Password Required',
    }),
    newPasswd: z.string().trim().min(8, {
      message: 'password must be at least 8 characters',
    }),
    confirmPasswd: z.string().trim().min(8, {
      message: 'password must be at least 8 characters',
    }),
  })
  .refine((data) => data.newPasswd === data.confirmPasswd, {
    message: 'The new password that you entered do not match!',
    path: ['confirmPasswd'],
  });

export default function Profile() {
  const [avatarFile, setAvatarFile] = useState<File | null>(null);
  const [avatarStr, setAvatarStr] = useState(''); // Avatar Image base64
  const { data: userInfo } = useFetchUserInfo();

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
  }, [userInfo]);

  useEffect(() => {
    if (avatarFile) {
      const fr = new FileReader();
      fr.onload = () => {
        setAvatarStr(fr.result as string);
      };
      fr.readAsDataURL(avatarFile);
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
    // final submit form
    console.log('data=', data);
  }

  return (
    <section className="p-8">
      <h1 className="text-3xl font-bold">Profile</h1>
      <div className="text-sm text-muted-foreground mb-6">
        Update your photo and personal details here.
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
                      <span className="text-red-600">*</span>User Name
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
                          {!avatarFile ? (
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
                                  src={avatarStr}
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
                      <span className="text-red-600">*</span>Time Zone
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
                        Email Address
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
                      Once registered, E-mail cannot be changed.
                    </p>
                  </div>
                </div>
              )}
            />
            <div className="h-[10px]"></div>
            <div className="pb-6">
              <h1 className="text-3xl font-bold">Password</h1>
              <div className="text-sm text-muted-foreground">
                Please enter your current password to change your password.
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
                      <span className="text-red-600">*</span>Current Password
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
                      <span className="text-red-600">*</span>New Password
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
                      <span className="text-red-600">*</span>Confirm password
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
              <Button variant="secondary">Cancel</Button>
              <Button type="submit">Save</Button>
            </div>
          </form>
        </Form>
      </div>
    </section>
  );
}
