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
  }
>(({ name, avatar, ...props }, ref) => {
  const index = random(0, 3);
  console.log('🚀 ~ index:', index);
  const value = Colors[index];
  return (
    <Avatar ref={ref} {...props}>
      <AvatarImage src={avatar} />
      <AvatarFallback
        className={`bg-gradient-to-b from-[${value.from}] to-[${value.to}]`}
      >
        {name?.slice(0, 1)}
      </AvatarFallback>
    </Avatar>
  );
});

RAGFlowAvatar.displayName = 'RAGFlowAvatar';
