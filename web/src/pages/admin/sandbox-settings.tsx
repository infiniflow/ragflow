import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import {
  LucideCloud,
  LucideLink,
  LucideLoader2,
  LucideSave,
  LucideServer,
  LucideZap,
} from 'lucide-react';

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
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';

import {
  getSandboxConfig,
  getSandboxProviderSchema,
  listSandboxProviders,
  setSandboxConfig,
  testSandboxConnection,
} from '@/services/admin-service';

import Spotlight from '@/components/spotlight';
import message from '@/components/ui/message';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { ScrollArea } from '@/components/ui/scroll-area';

// Provider icons mapping
const PROVIDER_ICONS: Record<string, React.ElementType> = {
  self_managed: LucideServer,
  aliyun_codeinterpreter: LucideCloud,
  e2b: LucideZap,
};

function AdminSandboxSettings() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  // State
  const [selectedProvider, setSelectedProvider] = useState<string | null>(null);
  const [configValues, setConfigValues] = useState<Record<string, unknown>>({});
  const [testModalOpen, setTestModalOpen] = useState(false);
  const [testResult, setTestResult] = useState<{
    success: boolean;
    message: string;
    details?: {
      exit_code: number;
      execution_time: number;
      stdout: string;
      stderr: string;
    };
  } | null>(null);

  // Fetch providers list
  const { data: providers = [], isLoading: providersLoading } = useQuery({
    queryKey: ['admin/listSandboxProviders'],
    queryFn: async () => (await listSandboxProviders()).data.data,
  });

  // Fetch current config
  const {
    data: currentConfig,
    isLoading: configLoading,
    refetch: refetchConfig,
  } = useQuery({
    queryKey: ['admin/getSandboxConfig'],
    queryFn: async () => (await getSandboxConfig()).data.data,
  });

  // Fetch provider schema when provider is selected
  const { data: providerSchema = {} } = useQuery({
    queryKey: ['admin/getSandboxProviderSchema', selectedProvider],
    queryFn: async () =>
      (await getSandboxProviderSchema(selectedProvider!)).data.data,
    enabled: !!selectedProvider,
  });

  // Set config mutation
  const setConfigMutation = useMutation({
    mutationFn: async (params: {
      providerType: string;
      config: Record<string, unknown>;
    }) => (await setSandboxConfig(params)).data,
    onSuccess: () => {
      message.success('Sandbox configuration updated successfully');
      // queryClient.invalidateQueries({ queryKey: ['admin/getSandboxConfig'] });
      refetchConfig();
    },
    onError: (error: Error) => {
      message.error(`Failed to update configuration: ${error.message}`);
    },
  });

  // Test connection mutation
  const testConnectionMutation = useMutation({
    mutationFn: async (params: {
      providerType: string;
      config: Record<string, unknown>;
    }) => (await testSandboxConnection(params)).data.data,
    onSuccess: (data) => {
      setTestResult(data);
    },
    onError: (error: Error) => {
      setTestResult({ success: false, message: error.message });
    },
  });

  // Initialize state when current config is loaded
  useEffect(() => {
    if (currentConfig) {
      setSelectedProvider(currentConfig.provider_type);
      setConfigValues(currentConfig.config || {});
    }
  }, [currentConfig]);

  // Apply schema defaults when provider schema changes
  useEffect(() => {
    if (providerSchema && Object.keys(providerSchema).length > 0) {
      setConfigValues((prev) => {
        const mergedConfig = { ...prev };
        // Apply schema defaults for any missing fields
        Object.entries(providerSchema).forEach(([fieldName, schema]) => {
          if (
            mergedConfig[fieldName] === undefined &&
            schema.default !== undefined
          ) {
            mergedConfig[fieldName] = schema.default;
          }
        });
        return mergedConfig;
      });
    }
  }, [providerSchema]);

  // Handle provider change
  const handleProviderChange = (providerId: string) => {
    setSelectedProvider(providerId);
    // Force refetch config and schema from backend when switching providers
    queryClient.invalidateQueries({ queryKey: ['admin/getSandboxConfig'] });
    queryClient.invalidateQueries({
      queryKey: ['admin/getSandboxProviderSchema'],
    });
  };

  // Handle config value change
  const handleConfigValueChange = (fieldName: string, value: unknown) => {
    setConfigValues((prev) => ({ ...prev, [fieldName]: value }));
  };

  // Handle save
  const handleSave = () => {
    if (!selectedProvider) return;

    setConfigMutation.mutate({
      providerType: selectedProvider,
      config: configValues,
    });
  };

  // Handle test connection
  const handleTestConnection = () => {
    if (!selectedProvider) return;

    setTestModalOpen(true);
    setTestResult(null);
    testConnectionMutation.mutate({
      providerType: selectedProvider,
      config: configValues,
    });
  };

  // Render config field based on schema
  const renderConfigField = (
    fieldName: string,
    schema: AdminService.SandboxConfigField,
  ) => {
    const value = configValues[fieldName] ?? schema.default ?? '';

    switch (schema.type) {
      case 'string':
        if (schema.secret) {
          return (
            <Input
              type="password"
              id={fieldName}
              className="h-10"
              placeholder={schema.placeholder}
              value={value as string}
              onChange={(e) =>
                handleConfigValueChange(fieldName, e.target.value)
              }
            />
          );
        }
        return (
          <Textarea
            id={fieldName}
            placeholder={schema.placeholder}
            value={value as string}
            onChange={(e) => handleConfigValueChange(fieldName, e.target.value)}
            rows={
              schema.description?.includes('endpoint') ||
              schema.description?.includes('URL')
                ? 1
                : 3
            }
          />
        );

      case 'integer':
        return (
          <Input
            type="number"
            id={fieldName}
            min={schema.min}
            max={schema.max}
            value={value as number}
            className="h-10"
            onChange={(e) =>
              handleConfigValueChange(fieldName, parseInt(e.target.value) || 0)
            }
          />
        );

      case 'boolean':
        return (
          <Switch
            id={fieldName}
            checked={value as boolean}
            onCheckedChange={(checked) =>
              handleConfigValueChange(fieldName, checked)
            }
          />
        );

      default:
        return null;
    }
  };

  const selectedProviderData = providers.find((p) => p.id === selectedProvider);
  const ProviderIcon = selectedProvider
    ? PROVIDER_ICONS[selectedProvider] || LucideServer
    : LucideServer;

  return (
    <>
      <Card className="!shadow-none relative h-full bg-transparent overflow-hidden">
        <Spotlight />

        <ScrollArea className="size-full">
          <CardHeader className="space-y-0">
            <CardTitle className="leading-10">
              {t('admin.sandboxSettings')}
            </CardTitle>
            <CardDescription className="text-text-secondary">
              {t('admin.sandboxSettingsPage.description')}
            </CardDescription>
          </CardHeader>

          <CardContent>
            {providersLoading || configLoading ? (
              <div className="flex items-center justify-center h-96"></div>
            ) : (
              <>
                {/* Provider Selection */}
                <Card className="!shadow-none bg-transparent">
                  <CardHeader>
                    <CardTitle className="text-xl leading-none">
                      {t('admin.sandboxSettingsPage.providerSelection')}
                    </CardTitle>

                    <CardDescription className="text-text-secondary">
                      {t(
                        'admin.sandboxSettingsPage.providerSelectionDescription',
                      )}
                    </CardDescription>

                    <RadioGroup
                      className="!mt-4 max-w-7xl grid grid-cols-1 xl:grid-cols-3 gap-4"
                      value={selectedProvider}
                      onValueChange={handleProviderChange}
                    >
                      {providers.map((provider) => {
                        const Icon =
                          PROVIDER_ICONS[provider.id] || LucideServer;

                        return (
                          <label
                            key={provider.id}
                            tabIndex={0}
                            className="
                              group relative rounded-lg border-0.5 border-border-button p-4 cursor-pointer transition-all
                              hover:bg-bg-card focus-visible:bg-bg-card
                              has-[[aria-checked=true]]:bg-bg-card
                              has-[[aria-checked=true]]:ring-1 has-[[aria-checked=true]]:ring-border-default"
                          >
                            <RadioGroupItem
                              key={provider.id}
                              className="hidden"
                              value={provider.id}
                            />

                            <div className="flex items-start gap-2">
                              <Icon className="size-5 transition-colors text-text-primary group-has-[[aria-checked=true]]:text-accent-primary" />

                              <div className="flex-1 min-w-0">
                                <h4 className="text-sm font-medium">
                                  {provider.name}
                                </h4>

                                <p className="text-xs text-text-secondary mt-1">
                                  {provider.description}
                                </p>

                                <div className="flex flex-wrap gap-1 mt-2">
                                  {provider.tags.map((tag) => (
                                    <span
                                      key={tag}
                                      className="inline-flex items-center px-2 py-0.5 rounded text-xs bg-bg-card text-text-secondary"
                                    >
                                      {tag}
                                    </span>
                                  ))}
                                </div>
                              </div>
                            </div>
                          </label>
                        );
                      })}
                    </RadioGroup>
                  </CardHeader>

                  {/* Provider Configuration */}
                  {selectedProvider && selectedProviderData && (
                    <CardContent className="pt-6 border-t-0.5 border-border-button">
                      <form onSubmit={handleSave}>
                        <article>
                          <header className="mb-6 flex items-center gap-4">
                            <div>
                              <h3 className="flex items-center gap-2">
                                <ProviderIcon className="size-[1em]" />

                                {t(
                                  'admin.sandboxSettingsPage.namedProviderConfiguration',
                                  { name: selectedProviderData.name },
                                )}
                              </h3>

                              <p className="text-sm text-text-secondary">
                                {t(
                                  'admin.sandboxSettingsPage.namedProviderConfigurationDescription',
                                  { name: selectedProviderData.name },
                                )}
                              </p>
                            </div>

                            <div className="ml-auto flex items-center gap-4">
                              <Button
                                onClick={handleTestConnection}
                                disabled={testConnectionMutation.isPending}
                                variant="outline"
                              >
                                {testConnectionMutation.isPending ? (
                                  <>
                                    <LucideLoader2 className="w-4 h-4 mr-2 animate-spin" />
                                    {t('admin.sandboxSettingsPage.testing')}
                                  </>
                                ) : (
                                  <>
                                    <LucideLink />
                                    {t(
                                      'admin.sandboxSettingsPage.testConnection',
                                    )}
                                  </>
                                )}
                              </Button>

                              <Button
                                type="submit"
                                disabled={setConfigMutation.isPending}
                              >
                                {setConfigMutation.isPending ? (
                                  <LucideLoader2 className="animate-spin" />
                                ) : (
                                  <LucideSave />
                                )}
                                {t('common.save')}
                              </Button>
                            </div>
                          </header>

                          <div className="space-y-4">
                            {Object.entries(providerSchema).map(
                              ([fieldName, schema]) => (
                                <div key={fieldName}>
                                  <Label
                                    htmlFor={fieldName}
                                    className="text-text-primary"
                                  >
                                    {schema.required && (
                                      <span className="text-state-error">
                                        *
                                      </span>
                                    )}
                                    {schema.label || fieldName}

                                    {schema.description && (
                                      <p className="text-xs text-text-secondary">
                                        {schema.description}
                                      </p>
                                    )}
                                  </Label>

                                  <div className="mt-2">
                                    {renderConfigField(fieldName, schema)}
                                  </div>

                                  {schema.type === 'integer' &&
                                    (schema.min !== undefined ||
                                      schema.max !== undefined) && (
                                      <p className="text-xs text-text-disabled mt-2">
                                        {schema.min !== undefined &&
                                          `Minimum: ${schema.min}`}
                                        {schema.min !== undefined &&
                                          schema.max !== undefined &&
                                          ' • '}
                                        {schema.max !== undefined &&
                                          `Maximum: ${schema.max}`}
                                      </p>
                                    )}
                                </div>
                              ),
                            )}
                          </div>
                        </article>
                      </form>
                    </CardContent>
                  )}
                </Card>
              </>
            )}
          </CardContent>
        </ScrollArea>
      </Card>

      {/* Test Result Modal */}
      <Dialog open={testModalOpen} onOpenChange={setTestModalOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {t('admin.sandboxSettingsPage.testConnectionResultModal.title')}
            </DialogTitle>
          </DialogHeader>

          <DialogDescription>
            {testResult === null
              ? t('admin.sandboxSettingsPage.testConnectionResultModal.testing')
              : testResult.success
                ? t(
                    'admin.sandboxSettingsPage.testConnectionResultModal.success',
                  )
                : t(
                    'admin.sandboxSettingsPage.testConnectionResultModal.failed',
                  )}
          </DialogDescription>

          {testResult === null ? (
            <div className="flex items-center justify-center py-8">
              <LucideLoader2 className="w-8 h-8 animate-spin text-muted-foreground" />
            </div>
          ) : (
            <div className="space-y-4">
              {/* Summary message */}
              <div
                className={`p-4 rounded-lg ${
                  testResult.success
                    ? 'bg-green-50 text-green-900 dark:bg-green-900/20 dark:text-green-100'
                    : 'bg-red-50 text-red-900 dark:bg-red-900/20 dark:text-red-100'
                }`}
              >
                <p className="text-sm whitespace-pre-wrap">
                  {testResult.message}
                </p>
              </div>

              {/* Detailed execution results */}
              {testResult.details && (
                <div className="space-y-3">
                  {/* Exit code and execution time */}
                  <div className="grid grid-cols-2 gap-2 text-sm">
                    <div className="p-2 bg-muted rounded">
                      <span className="font-medium">
                        {t(
                          'admin.sandboxSettingsPage.testConnectionResultModal.exitCode',
                        )}
                        :
                      </span>{' '}
                      {testResult.details.exit_code}
                    </div>
                    <div className="p-2 bg-muted rounded">
                      <span className="font-medium">
                        {t(
                          'admin.sandboxSettingsPage.testConnectionResultModal.executionTime',
                        )}
                        :
                      </span>{' '}
                      {testResult.details.execution_time?.toFixed(2)}s
                    </div>
                  </div>

                  {/* Standard output */}
                  {testResult.details.stdout && (
                    <div className="p-3 bg-muted rounded">
                      <p className="text-xs font-medium mb-2 text-muted-foreground">
                        {t(
                          'admin.sandboxSettingsPage.testConnectionResultModal.stdout',
                        )}
                        :
                      </p>
                      <pre className="text-xs whitespace-pre-wrap break-words font-mono">
                        {testResult.details.stdout}
                      </pre>
                    </div>
                  )}

                  {/* Standard error (stack traces) */}
                  {testResult.details.stderr && (
                    <div className="p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded">
                      <p className="text-xs font-medium mb-2 text-red-900 dark:text-red-100">
                        {t(
                          'admin.sandboxSettingsPage.testConnectionResultModal.stderr',
                        )}
                        :
                      </p>
                      <pre className="text-xs whitespace-pre-wrap break-words font-mono text-red-900 dark:text-red-100">
                        {testResult.details.stderr}
                      </pre>
                    </div>
                  )}
                </div>
              )}
            </div>
          )}
          <DialogFooter>
            <Button onClick={() => setTestModalOpen(false)}>
              {t('admin.close')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

export default AdminSandboxSettings;
