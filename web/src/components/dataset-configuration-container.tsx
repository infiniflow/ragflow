import { PropsWithChildren } from 'react';

export function DatasetConfigurationContainer({ children }: PropsWithChildren) {
  return (
    <div className="border p-2 rounded-lg bg-slate-50 dark:bg-gray-600">
      {children}
    </div>
  );
}
