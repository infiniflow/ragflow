import { BackendVariant } from '@/utils/backend-variant';
import { lazy, Suspense } from 'react';

const GoDatasetSetting = lazy(() => import('./go'));
const PythonDatasetSetting = lazy(() => import('./python'));

// Single configuration route for both backends. Each variant page stays in
// its own chunk so a deployment only ever downloads the implementation its
// backend serves.
export default function DatasetSettingPage() {
  return (
    <Suspense fallback={null}>
      <BackendVariant
        go={<GoDatasetSetting />}
        python={<PythonDatasetSetting />}
      />
    </Suspense>
  );
}
