import { cn } from '@/lib/utils';
import * as AvatarPrimitive from '@radix-ui/react-avatar';
import { random } from 'lodash';
import { forwardRef } from 'react';
import { Avatar, AvatarFallback, AvatarImage } from './ui/avatar';

const Colors = [
  { from: '#4F6DEE', to: '#67BDF9' },
  { from: '#38A04D', to: '#93DCA2' },
  { from: '#EDB395', to: '#C35F2B' },
  { from: '#633897', to: '#CBA1FF' },
];

export const RAGFlowAvatar = forwardRef<
  React.ElementRef<typeof AvatarPrimitive.Root>,
  React.ComponentPropsWithoutRef<typeof AvatarPrimitive.Root> & {
    name?: string;
    avatar?: string;
    isPerson?: boolean;
  }
>(({ name, avatar, isPerson = false, className, ...props }, ref) => {
  const index = random(0, 3);
  console.log('🚀 ~ index:', index);
  const value = Colors[index];
  return (
    <Avatar
      ref={ref}
      {...props}
      className={cn(className, { 'rounded-md': !isPerson })}
    >
      <AvatarImage src={avatar} />
      <AvatarFallback
        className={cn(
          `bg-gradient-to-b from-[${value.from}] to-[${value.to}]`,
          { 'rounded-md': !isPerson },
        )}
      >
        {name?.slice(0, 1)}
      </AvatarFallback>
    </Avatar>
  );
});

RAGFlowAvatar.displayName = 'RAGFlowAvatar';
