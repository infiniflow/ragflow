// registry/default/components/comp-430.tsx

import { cn } from '@/lib/utils';
import * as TabsPrimitive from '@radix-ui/react-tabs';
import React from 'react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../ui/tabs';

export const UnderlineTabsList = React.forwardRef<
  React.ElementRef<typeof TabsPrimitive.List>,
  React.ComponentPropsWithoutRef<typeof TabsPrimitive.List>
>(function UnderlineTabsList({ className, ...props }, ref) {
  return (
    <TabsList
      ref={ref}
      className={cn(
        'text-foreground h-auto gap-2 rounded-none border-b bg-transparent px-0 py-1',
        className,
      )}
      {...props}
    />
  );
});

export const UnderlineTabsTrigger = React.forwardRef<
  React.ElementRef<typeof TabsPrimitive.Trigger>,
  React.ComponentPropsWithoutRef<typeof TabsPrimitive.Trigger>
>(function UnderlineTabsTrigger({ className, ...props }, ref) {
  return (
    <TabsTrigger
      ref={ref}
      className={cn(
        'hover:bg-accent  hover:text-foreground data-[state=active]:after:bg-primary data-[state=active]:hover:bg-accent relative after:absolute after:inset-x-0 after:bottom-0 after:-mb-1 after:h-0.5 data-[state=active]:bg-transparent data-[state=active]:shadow-none',
        className,
      )}
      {...props}
    />
  );
});

export { Tabs as UnderlineTabs, TabsContent as UnderlineTabsContent };
