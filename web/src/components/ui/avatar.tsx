'use client';

import * as AvatarPrimitive from '@radix-ui/react-avatar';
import * as React from 'react';

import { cn } from '@/lib/utils';

const Avatar = React.forwardRef<
  React.ElementRef<typeof AvatarPrimitive.Root>,
  React.ComponentPropsWithoutRef<typeof AvatarPrimitive.Root>
>(({ className, ...props }, ref) => (
  <AvatarPrimitive.Root
    ref={ref}
    className={cn(
      'relative flex h-10 w-10 shrink-0 overflow-hidden rounded-full',
      className,
    )}
    {...props}
  />
));
Avatar.displayName = AvatarPrimitive.Root.displayName;

const AvatarImage = React.forwardRef<
  React.ElementRef<typeof AvatarPrimitive.Image>,
  React.ComponentPropsWithoutRef<typeof AvatarPrimitive.Image>
>(({ className, ...props }, ref) => (
  <AvatarPrimitive.Image
    ref={ref}
    className={cn('aspect-square h-full w-full', className)}
    {...props}
  />
));
AvatarImage.displayName = AvatarPrimitive.Image.displayName;

const AvatarFallback = React.forwardRef<
  React.ElementRef<typeof AvatarPrimitive.Fallback>,
  React.ComponentPropsWithoutRef<typeof AvatarPrimitive.Fallback>
>(({ className, ...props }, ref) => (
  <AvatarPrimitive.Fallback
    ref={ref}
    className={cn(
      'flex h-full w-full items-center justify-center rounded-full bg-bg-member',
      className,
    )}
    {...props}
  />
));
AvatarFallback.displayName = AvatarPrimitive.Fallback.displayName;

export { Avatar, AvatarFallback, AvatarImage };

type AvatarProps = React.ComponentProps<typeof Avatar>;

interface AvatarGroupProps extends React.ComponentProps<'div'> {
  children: React.ReactElement<AvatarProps>[];
  max?: number;
}

export const AvatarGroup = ({
  children,
  max,
  className,
  ...props
}: AvatarGroupProps) => {
  const totalAvatars = React.Children.count(children);
  const displayedAvatars = React.Children.toArray(children)
    .slice(0, max)
    .reverse();
  const remainingAvatars = max ? Math.max(totalAvatars - max, 1) : 0;

  return (
    <div
      className={cn('flex items-center flex-row-reverse', className)}
      {...props}
    >
      {remainingAvatars > 0 && (
        <Avatar className="-ml-2 hover:z-10 relative ring-2 ring-background">
          <AvatarFallback className="bg-muted-foreground text-white">
            +{remainingAvatars}
          </AvatarFallback>
        </Avatar>
      )}
      {displayedAvatars.map((avatar, index) => {
        if (!React.isValidElement(avatar)) return null;

        return (
          <div key={index} className="-ml-2 hover:z-10 relative">
            {React.cloneElement(avatar as React.ReactElement<AvatarProps>, {
              className: 'ring-2 ring-background',
            })}
          </div>
        );
      })}
    </div>
  );
};
