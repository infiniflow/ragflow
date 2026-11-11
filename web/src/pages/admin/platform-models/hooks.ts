import {
  useIsMutating,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import { useCallback, useLayoutEffect, useRef } from 'react';
import type { SyntheticEvent } from 'react';
import type { AxiosResponse } from 'axios';
import { LlmKeys } from '@/hooks/use-llm-request';

import {
  addPlatformModel,
  checkPlatformConnection,
  createPlatformInstance,
  createPlatformProvider,
  discoverPlatformModels,
  getPlatformInstance,
  listPlatformDefaultModels,
  listPlatformInstances,
  listPlatformModels,
  listPlatformProviderCatalog,
  listPlatformProviders,
  setPlatformDefaultModel,
  updatePlatformInstance,
  updatePlatformModel,
} from '@/services/admin-service';

export const PlatformModelKeys = {
  all: () => ['admin/platformModels'] as const,
  providers: () => [...PlatformModelKeys.all(), 'providers'] as const,
  catalog: () => [...PlatformModelKeys.all(), 'catalog'] as const,
  instances: (providerId: string) =>
    [...PlatformModelKeys.all(), 'instances', providerId] as const,
  instance: (providerId: string, instanceId: string) =>
    [...PlatformModelKeys.all(), 'instance', providerId, instanceId] as const,
  models: (providerId: string, instanceId: string) =>
    [...PlatformModelKeys.all(), 'models', providerId, instanceId] as const,
  allModels: (providerIds: string[] = []) =>
    [...PlatformModelKeys.all(), 'allModels', providerIds] as const,
  defaults: () => [...PlatformModelKeys.all(), 'defaults'] as const,
  saveInstance: (providerId: string) =>
    [...PlatformModelKeys.all(), 'saveInstance', providerId] as const,
  saveModel: (providerId: string, instanceId: string) =>
    [...PlatformModelKeys.all(), 'saveModel', providerId, instanceId] as const,
  checkConnection: (providerId: string) =>
    [...PlatformModelKeys.all(), 'checkConnection', providerId] as const,
  discover: (providerId: string) =>
    [...PlatformModelKeys.all(), 'discover', providerId] as const,
};

type Envelope<T> = { code: number; message: string; data: T };

/**
 * The admin request layer already reports non-zero codes to the user. Throw
 * so mutations stay in their pending/error state and dialogs keep the input.
 */
async function unwrap<T>(request: Promise<AxiosResponse<Envelope<T>>>) {
  const { data } = await request;
  if (data?.code !== 0) {
    throw new Error(data?.message);
  }
  return data.data;
}

export type PlatformModelOption = AdminService.PlatformModel & {
  provider_id: string;
  provider_name: string;
  instance_id: string;
  instance_name: string;
  instance_status: AdminService.PlatformStatus;
};

export function usePlatformProviders() {
  return useQuery({
    // The admin request layer reports each failure; retrying repeats it.
    retry: false,
    queryKey: PlatformModelKeys.providers(),
    queryFn: async () => (await unwrap(listPlatformProviders())) ?? [],
  });
}

export function usePlatformProviderCatalog() {
  return useQuery({
    // The admin request layer reports each failure; retrying repeats it.
    retry: false,
    queryKey: PlatformModelKeys.catalog(),
    queryFn: async () => (await unwrap(listPlatformProviderCatalog())) ?? [],
    staleTime: Infinity,
  });
}

export function usePlatformInstances(providerId: string) {
  return useQuery({
    // The admin request layer reports each failure; retrying repeats it.
    retry: false,
    queryKey: PlatformModelKeys.instances(providerId),
    queryFn: async () =>
      (await unwrap(listPlatformInstances(providerId))) ?? [],
    enabled: Boolean(providerId),
  });
}

export function usePlatformInstanceDetail(
  providerId: string,
  instanceId: string | undefined,
) {
  return useQuery({
    // The admin request layer reports each failure; retrying repeats it.
    retry: false,
    queryKey: PlatformModelKeys.instance(providerId, instanceId ?? ''),
    queryFn: () => unwrap(getPlatformInstance(providerId, instanceId!)),
    enabled: Boolean(providerId && instanceId),
    gcTime: 0,
  });
}

export function usePlatformModels(
  providerId: string,
  instanceId: string,
  enabled = true,
) {
  return useQuery({
    // The admin request layer reports each failure; retrying repeats it.
    retry: false,
    queryKey: PlatformModelKeys.models(providerId, instanceId),
    queryFn: async () =>
      (await unwrap(listPlatformModels(providerId, instanceId))) ?? [],
    enabled,
  });
}

/**
 * Every platform model with its location, for the default-model pickers.
 * Built on the provider list the page already loads, so one failure of that
 * list is reported once and one retry recovers both.
 */
export function useAllPlatformModels() {
  const providersQuery = usePlatformProviders();
  const providers = providersQuery.data;
  const modelsQuery = useQuery({
    // The admin request layer reports each failure; retrying repeats it.
    retry: false,
    queryKey: PlatformModelKeys.allModels(
      providers?.map((provider) => provider.id) ?? [],
    ),
    enabled: Boolean(providers),
    queryFn: async () => {
      const perProvider = await Promise.all(
        (providers ?? []).map(async (provider) => {
          const instances =
            (await unwrap(listPlatformInstances(provider.id))) ?? [];
          const perInstance = await Promise.all(
            instances.map(async (instance) => {
              const models =
                (await unwrap(listPlatformModels(provider.id, instance.id))) ??
                [];
              return models.map<PlatformModelOption>((model) => ({
                ...model,
                provider_id: provider.id,
                provider_name: provider.name,
                instance_id: instance.id,
                instance_name: instance.instance_name,
                instance_status: instance.status,
              }));
            }),
          );
          return perInstance.flat();
        }),
      );
      return perProvider.flat();
    },
  });
  return { modelsQuery, providersQuery };
}

export function usePlatformDefaults() {
  return useQuery({
    // The admin request layer reports each failure; retrying repeats it.
    retry: false,
    queryKey: PlatformModelKeys.defaults(),
    queryFn: async () =>
      (await unwrap(listPlatformDefaultModels()))?.models ?? [],
  });
}

/** Refresh platform configuration after a change; the catalog is static. */
function useInvalidatePlatformModels() {
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({
        queryKey: PlatformModelKeys.all(),
        predicate: (query) =>
          query.queryKey[1] !== PlatformModelKeys.catalog()[1],
      }),
      queryClient.invalidateQueries({ queryKey: LlmKeys.defaultModels() }),
    ]);
}

export function useCreatePlatformProvider() {
  const invalidate = useInvalidatePlatformModels();
  return useMutation({
    mutationFn: (providerName: string) =>
      unwrap(createPlatformProvider(providerName)),
    onSuccess: invalidate,
  });
}

export function useSavePlatformInstance(providerId: string) {
  const invalidate = useInvalidatePlatformModels();
  return useMutation({
    mutationKey: PlatformModelKeys.saveInstance(providerId),
    mutationFn: ({
      instanceId,
      params,
    }: {
      instanceId?: string;
      params: AdminService.PlatformInstanceInput;
    }) =>
      instanceId
        ? unwrap(updatePlatformInstance(providerId, instanceId, params))
        : unwrap(createPlatformInstance(providerId, params)),
    onSuccess: invalidate,
  });
}

export function useSetPlatformInstanceStatus(providerId: string) {
  const invalidate = useInvalidatePlatformModels();
  return useMutation({
    mutationFn: ({
      instanceId,
      status,
    }: {
      instanceId: string;
      status: AdminService.PlatformStatus;
    }) => unwrap(updatePlatformInstance(providerId, instanceId, { status })),
    onSuccess: invalidate,
  });
}

export function useSavePlatformModel(providerId: string, instanceId: string) {
  const invalidate = useInvalidatePlatformModels();
  return useMutation({
    mutationKey: PlatformModelKeys.saveModel(providerId, instanceId),
    mutationFn: ({
      modelId,
      params,
    }: {
      modelId?: string;
      params: AdminService.PlatformModelInput;
    }) =>
      modelId
        ? unwrap(
            updatePlatformModel(providerId, instanceId, modelId, {
              model_type: params.model_type,
              max_tokens: params.max_tokens,
              extra: params.extra,
            }),
          )
        : unwrap(addPlatformModel(providerId, instanceId, params)),
    onSuccess: invalidate,
  });
}

export function useSetPlatformModelStatus(
  providerId: string,
  instanceId: string,
) {
  const invalidate = useInvalidatePlatformModels();
  return useMutation({
    mutationFn: ({
      modelId,
      status,
    }: {
      modelId: string;
      status: AdminService.PlatformStatus;
    }) =>
      unwrap(updatePlatformModel(providerId, instanceId, modelId, { status })),
    onSuccess: invalidate,
  });
}

export function useCheckPlatformConnection(providerId: string) {
  return useMutation({
    mutationKey: PlatformModelKeys.checkConnection(providerId),
    mutationFn: (params: AdminService.PlatformConnectionInput) =>
      unwrap(checkPlatformConnection(providerId, params)),
  });
}

export function useDiscoverPlatformModels(providerId: string) {
  return useMutation({
    mutationKey: PlatformModelKeys.discover(providerId),
    mutationFn: async (params: AdminService.PlatformConnectionInput) =>
      (await unwrap(discoverPlatformModels(providerId, params))) ?? [],
  });
}

export function useSetPlatformDefault() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      modelType,
      modelId,
    }: {
      modelType: string;
      modelId: string | null;
    }) => unwrap(setPlatformDefaultModel(modelType, modelId)),
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({
          queryKey: PlatformModelKeys.defaults(),
        }),
        queryClient.invalidateQueries({ queryKey: LlmKeys.defaultModels() }),
      ]),
  });
}

/**
 * These dialogs open from plain buttons, not a Radix trigger, so Radix has
 * nothing to return focus to when they close. Remember what had focus when
 * the dialog opened and hand focus back to it, if it is still on the page.
 * Pass the result to DialogContent's onCloseAutoFocus.
 */
export function useReturnFocusOnClose(open: boolean) {
  const openerRef = useRef<HTMLElement | null>(null);
  // Runs before the dialog moves focus into itself.
  useLayoutEffect(() => {
    if (open) {
      const active = document.activeElement;
      openerRef.current = active instanceof HTMLElement ? active : null;
    }
  }, [open]);
  return useCallback((event: Event) => {
    const opener = openerRef.current;
    if (opener?.isConnected) {
      event.preventDefault();
      opener.focus();
    }
  }, []);
}

/**
 * The dialog's open handler, refusing to close while one of `mutationKeys`
 * runs: a save closes the dialog when it succeeds, which would otherwise
 * close a dialog opened again meanwhile, and a reopened form would start a
 * second request next to the running one.
 */
export function useCloseUnlessSaving(
  onOpenChange: (open: boolean) => void,
  mutationKeys: readonly (readonly unknown[])[],
) {
  const saving =
    useIsMutating({
      predicate: (mutation) =>
        mutationKeys.some((key) =>
          key.every((part, i) => mutation.options.mutationKey?.[i] === part),
        ),
    }) > 0;
  return useCallback(
    (open: boolean) => {
      if (!open && saving) return;
      onOpenChange(open);
    },
    [onOpenChange, saving],
  );
}

/**
 * Runs `action` once at a time. The check is synchronous, so a second click
 * in the same frame, before a re-render, is ignored too.
 */
export function useSingleFlight<A extends unknown[]>(
  action: (...args: A) => Promise<unknown>,
) {
  const runningRef = useRef(false);
  return useCallback(
    async (...args: A) => {
      if (runningRef.current) return;
      runningRef.current = true;
      try {
        await action(...args);
      } finally {
        runningRef.current = false;
      }
    },
    [action],
  );
}

// Disabled fieldsets do not block custom controls or portal events.
export function useBlockBusyEvents(busy: boolean) {
  const block = useCallback(
    (event: SyntheticEvent) => {
      if (busy) {
        event.preventDefault();
        event.stopPropagation();
      }
    },
    [busy],
  );
  return {
    onPointerDownCapture: block,
    onPointerUpCapture: block,
    onClickCapture: block,
    onKeyDownCapture: block,
  };
}
