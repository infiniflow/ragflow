import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { LucidePlus } from 'lucide-react';

import { SelectWithSearch } from '@/components/originui/select-with-search';
import Spotlight from '@/components/spotlight';
import { LlmIcon } from '@/components/svg-icon';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import message from '@/components/ui/message';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Skeleton } from '@/components/ui/skeleton';

import { BusyButton } from './busy-button';
import { UnsupportedPlatformProviders } from './constants';
import { PlatformDefaultsSection } from './defaults-section';
import {
  useCreatePlatformProvider,
  usePlatformProviderCatalog,
  usePlatformProviders,
  useReturnFocusOnClose,
  useSingleFlight,
} from './hooks';
import { LoadError } from './load-error';
import { ProviderSection } from './provider-section';

/**
 * Platform model providers shared by every tenant. Tenants can use these
 * models but never see their credentials or change their configuration.
 */
function AdminPlatformModels() {
  const { t } = useTranslation();
  const {
    data: providers = [],
    isLoading,
    isError,
    refetch,
  } = usePlatformProviders();
  const [addOpen, setAddOpen] = useState(false);
  const handleAdd = useCallback(() => setAddOpen(true), []);
  const handleRetry = useCallback(() => refetch(), [refetch]);
  const existingNames = useMemo(
    () => providers.map((provider) => provider.name),
    [providers],
  );

  return (
    <Card className="!shadow-none relative h-full bg-transparent overflow-hidden">
      <Spotlight />

      <ScrollArea className="size-full" viewportClassName="[&>div]:!block">
        <CardHeader className="space-y-0 flex flex-row flex-wrap gap-4 justify-between items-center">
          <div className="min-w-0">
            <CardTitle className="leading-10">
              {t('admin.platformModels')}
            </CardTitle>
            <CardDescription className="text-text-secondary">
              {t('admin.platformModelsPage.description')}
            </CardDescription>
          </div>
          <Button className="h-10 px-4" onClick={handleAdd}>
            <LucidePlus />
            {t('admin.platformModelsPage.addProvider')}
          </Button>
        </CardHeader>

        <CardContent className="space-y-6">
          <PlatformDefaultsSection />

          <Card className="!shadow-none bg-transparent">
            <CardHeader>
              <CardTitle className="text-xl leading-none">
                {t('admin.platformModelsPage.providersTitle')}
              </CardTitle>
              <CardDescription className="text-text-secondary">
                {t('admin.platformModelsPage.providersDescription')}
              </CardDescription>
            </CardHeader>
            <CardContent className="max-w-6xl space-y-4">
              {isError ? (
                <LoadError onRetry={handleRetry} />
              ) : isLoading ? (
                <Skeleton className="h-24 w-full" />
              ) : providers.length === 0 ? (
                <p className="text-sm text-text-secondary">
                  {t('admin.platformModelsPage.noProviders')}
                </p>
              ) : (
                providers.map((provider) => (
                  <ProviderSection key={provider.id} provider={provider} />
                ))
              )}
            </CardContent>
          </Card>
        </CardContent>
      </ScrollArea>

      <AddProviderDialog
        open={addOpen}
        onOpenChange={setAddOpen}
        existingNames={existingNames}
      />
    </Card>
  );
}

function AddProviderDialog({
  open,
  onOpenChange,
  existingNames,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  existingNames: string[];
}) {
  const { t } = useTranslation();
  const returnFocus = useReturnFocusOnClose(open);
  const {
    data: catalog = [],
    isLoading: catalogLoading,
    isError: catalogFailed,
    refetch: refetchCatalog,
  } = usePlatformProviderCatalog();
  const handleRetry = useCallback(() => refetchCatalog(), [refetchCatalog]);
  const createMutation = useCreatePlatformProvider();
  const [selected, setSelected] = useState('');

  const options = useMemo(() => {
    const existing = new Set(existingNames);
    return catalog
      .filter(
        (item) =>
          !existing.has(item.name) &&
          !UnsupportedPlatformProviders.has(item.name),
      )
      .map((item) => ({
        label: item.name,
        value: item.name,
        icon: (
          <LlmIcon
            name={item.name}
            width={20}
            height={20}
            imgClass="size-5 shrink-0"
          />
        ),
      }));
  }, [catalog, existingNames]);

  // Reset on opening, so the closing dialog keeps showing its choice.
  const [wasOpen, setWasOpen] = useState(open);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) setSelected('');
  }
  // Closing waits for a running create, which closes the dialog itself.
  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next && createMutation.isPending) return;
      onOpenChange(next);
    },
    [onOpenChange, createMutation.isPending],
  );

  const handleConfirm = useSingleFlight(
    useCallback(async () => {
      if (!selected) return;
      try {
        await createMutation.mutateAsync(selected);
        message.success(t('message.created'));
        onOpenChange(false);
      } catch {
        // Reported by the admin request layer.
      }
    }, [selected, createMutation, onOpenChange, t]),
  );
  const handleCancel = useCallback(
    () => handleOpenChange(false),
    [handleOpenChange],
  );

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent onCloseAutoFocus={returnFocus}>
        <DialogHeader>
          <DialogTitle>{t('admin.platformModelsPage.addProvider')}</DialogTitle>
        </DialogHeader>
        <section className="px-6 space-y-4">
          <DialogDescription>
            {t('admin.platformModelsPage.addProviderDescription')}
          </DialogDescription>
          {/* A failed catalog must not look like one with no providers. */}
          {catalogFailed ? (
            <LoadError
              onRetry={handleRetry}
              message={t('admin.platformModelsPage.catalogLoadFailed')}
            />
          ) : catalogLoading ? (
            <Skeleton className="h-10 w-full" />
          ) : (
            <SelectWithSearch
              ariaLabel={t('admin.platformModelsPage.selectProvider')}
              value={selected}
              onChange={setSelected}
              options={options}
              placeholder={t('admin.platformModelsPage.selectProvider')}
              testId="platform-provider-select"
            />
          )}
        </section>
        <DialogFooter className="gap-4 px-6 py-4">
          <Button
            variant="outline"
            className="px-4 h-10 dark:border-border-button"
            onClick={handleCancel}
            disabled={createMutation.isPending}
          >
            {t('admin.cancel')}
          </Button>
          <BusyButton
            className="px-4 h-10"
            onClick={handleConfirm}
            disabled={!selected}
            busy={createMutation.isPending}
          >
            {t('admin.confirm')}
          </BusyButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default AdminPlatformModels;
