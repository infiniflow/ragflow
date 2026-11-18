import type { IntegrationChild } from '@/layouts/components/app-sidebar/navigation';
import { useVisibleIntegrationChildren } from '../use-visible-integration-children';

it('preserves all integration children in OSS', () => {
  const children: IntegrationChild[] = [
    { key: 'model', path: '/integrations/model', labelKey: 'sidebar.models' },
    {
      key: 'model-group',
      path: '/integrations/model-group',
      labelKey: 'setting.modelGroup',
    },
  ];

  expect(useVisibleIntegrationChildren(children)).toBe(children);
});
