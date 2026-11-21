/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

'use client';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import {
  verdictLabel,
  verdictReasons,
} from '@/components/compilation-template-group/ontology/action-verdict';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import message from '@/components/ui/message';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import {
  useDeleteActionBinding,
  useExecuteAction,
  useFetchModelMCPServers,
  useFetchModelMCPTools,
  useSaveActionBinding,
} from '@/hooks/use-ontology-action-request';
import {
  IActionBinding,
  IActionCallResult,
  IModelAction,
} from '@/interfaces/database/ontology-model';
import { ReactNode, useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

/** The mapping is stored as strings; anything else is normalised on submit. */
function mappingToText(mapping?: Record<string, string>) {
  if (!mapping || Object.keys(mapping).length === 0) {
    return '';
  }
  return JSON.stringify(mapping, null, 2);
}

function verdictTone(verdict?: string) {
  switch (verdict) {
    case 'allow':
      return 'text-state-success border-state-success';
    case 'confirm':
      return 'text-state-warning border-state-warning';
    case 'refuse':
      return 'text-state-error border-state-error';
    default:
      return 'text-text-secondary border-border-button';
  }
}

/**
 * The one way to try an action: name the tool, then run it once.
 *
 * There is deliberately no separate rehearsal button. A rehearsal and a run ask
 * the same question — can this action reach its tool with these arguments — and
 * two buttons made every reader pick one before they had seen the answer. The
 * run is audited either way, refusals included, so the audit trail already holds
 * what a rehearsal would have reported.
 *
 * The address is asked for here because a model can list several systems: the
 * model's list is offered as choices, the first being the default, and a binding
 * that has to reach something else can still type its own.
 */
export function ActionRunTest({
  templateId,
  action,
  binding,
  modelServers,
  execEnabled = true,
  trigger,
}: {
  templateId: string;
  action: IModelAction;
  binding?: IActionBinding;
  /** The model's addresses; the first is the default this action follows. */
  modelServers: string[];
  /**
   * The process-wide execution switch. When it is off the binding can still be
   * written — that is how a deployment prepares one before turning execution on
   * — but nothing is called, so the dialog offers saving instead of running.
   */
  execEnabled?: boolean;
  trigger: ReactNode;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  // The address this binding dials. Empty means "not chosen yet", which falls
  // back to the model's first entry so a fresh binding is never address-less.
  const [server, setServer] = useState('');
  const [tool, setTool] = useState('');
  const [mappingText, setMappingText] = useState('');
  const [readOnly, setReadOnly] = useState(false);
  // Whether an agent may call this tool while answering. Off by default: a
  // binding is a decision the operator makes, not something the agent grants
  // itself, and nothing else on this page can flip it.
  const [qaCallable, setQaCallable] = useState(false);
  const [riskAck, setRiskAck] = useState(false);
  const [reason, setReason] = useState('');
  const [result, setResult] = useState<IActionCallResult>();
  const { loading: saving, saveBinding } = useSaveActionBinding();
  const { loading: running, execute } = useExecuteAction();
  const { loading: deleting, deleteBinding } = useDeleteActionBinding();

  // Read here as well as on the page above the list: the same cached query, but
  // the dialog must still offer the addresses when the page's copy has not
  // loaded yet, otherwise "no address" is what the reader sees.
  const { servers: fetchedServers } = useFetchModelMCPServers(templateId, open);
  const knownServers = modelServers.length > 0 ? modelServers : fetchedServers;

  // What this action will dial: what the operator picked here, else the address
  // the stored binding carries, else the model's first entry.
  const endpoint = (
    server ||
    binding?.effective_mcp_server ||
    knownServers[0] ||
    ''
  ).trim();

  // The tools that address advertises. Read only while the dialog is open: the
  // call reaches the MCP server, and the actions page does not need it.
  const {
    tools,
    loading: toolsLoading,
    error: toolsError,
    refetch: refetchTools,
  } = useFetchModelMCPTools(templateId, endpoint, open && !!endpoint);

  const toolNames = useMemo(() => {
    const found = (tools ?? []).map((item) => item.name).filter(Boolean);
    // A stored binding may name a tool the server no longer lists. Keeping it
    // in the options shows what the row is wired to instead of blanking the
    // field and inviting a silent replacement.
    const current = tool.trim();
    return current && !found.includes(current) ? [current, ...found] : found;
  }, [tools, tool]);

  // A server that exposes exactly one tool leaves nothing to choose, so an
  // unbound action starts with it filled in: the binding is still per action,
  // but the operator no longer repeats the same pick for every one of them.
  useEffect(() => {
    if (!open || binding || tool) {
      return;
    }
    if (toolNames.length === 1) {
      setTool(toolNames[0]);
    }
  }, [open, binding, tool, toolNames]);

  // Which failure needs the operator to change a deployment setting rather than
  // the address: the guard refuses to dial private addresses until
  // ALLOW_ANY_HOST is on.
  const privateHostBlocked =
    (toolsError as (Error & { code?: string }) | null)?.code ===
    'MCP_PRIVATE_HOST_BLOCKED';

  const reset = useCallback(() => {
    // The stored binding's own address wins; empty means it followed the model,
    // and picking again starts from that default.
    setServer(binding?.mcp_server || binding?.effective_mcp_server || '');
    setTool(binding?.mcp_tool ?? '');
    setMappingText(mappingToText(binding?.arg_mapping));
    // Not declared read-only until someone says so: ticking it would let a write
    // action skip the confirmation a human would otherwise give.
    setReadOnly(binding?.read_only ?? false);
    setQaCallable(binding?.qa_callable ?? false);
    setRiskAck(false);
    setReason('');
    setResult(undefined);
  }, [binding]);

  const handleOpenChange = useCallback(
    (next: boolean) => {
      setOpen(next);
      if (next) {
        reset();
      }
    },
    [reset],
  );

  const parseMapping = useCallback((): Record<string, string> | undefined => {
    const text = mappingText.trim();
    if (!text) {
      return {};
    }
    let parsed: unknown;
    try {
      parsed = JSON.parse(text);
    } catch {
      message.error(
        t('knowledgeCompilation.ontologyInspect.actionMappingInvalid', {
          defaultValue: 'The argument mapping is not valid JSON.',
        }),
      );
      return undefined;
    }
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      message.error(
        t('knowledgeCompilation.ontologyInspect.actionMappingInvalid', {
          defaultValue: 'The argument mapping is not valid JSON.',
        }),
      );
      return undefined;
    }
    return Object.fromEntries(
      Object.entries(parsed as Record<string, unknown>).map(([key, value]) => [
        key,
        typeof value === 'string' ? value : JSON.stringify(value),
      ]),
    );
  }, [mappingText, t]);

  const handleRun = useCallback(async () => {
    const toolName = tool.trim();
    if (!endpoint) {
      message.error(
        t('knowledgeCompilation.ontologyInspect.actionModelEndpointRequired', {
          defaultValue:
            'Set the model MCP address above the action list first.',
        }),
      );
      return;
    }
    if (!toolName) {
      message.error(
        t('knowledgeCompilation.ontologyInspect.actionBindingRequired', {
          defaultValue: 'Pick an MCP server and a tool first.',
        }),
      );
      return;
    }
    const argMapping = parseMapping();
    if (!argMapping) {
      return;
    }
    const reasonText = reason.trim();
    // A reason is only required for a real call: with execution off nothing runs,
    // so demanding one would ask the operator to justify an attempt that cannot
    // happen.
    if (execEnabled && !reasonText) {
      message.error(
        t('knowledgeCompilation.ontologyInspect.actionReasonRequired', {
          defaultValue: 'State why this action is being run.',
        }),
      );
      return;
    }
    if (!readOnly && !riskAck) {
      message.error(
        t('knowledgeCompilation.ontologyInspect.actionBindingRiskRequired', {
          defaultValue:
            'Confirm the risk before running a tool that can change a system outside RAGFlow.',
        }),
      );
      return;
    }
    try {
      // The run reads its wiring from the stored binding, so the binding is saved
      // first. A rejected binding (missing tool, unmapped argument) stops here
      // with the server's reason rather than surfacing as a failed tool call.
      const saved = await saveBinding({
        id: templateId,
        actionName: action.name,
        bindingId: binding?.id,
        payload: {
          // The resolved address is stored, not a pointer to "the default": a
          // model may list several systems, so a binding that followed the list
          // would silently move to whatever the first line becomes.
          mcp_server: endpoint,
          mcp_tool: toolName,
          arg_mapping: argMapping,
          read_only: readOnly,
          enabled: true,
          qa_callable: qaCallable,
          risk_ack: riskAck,
          note: binding?.note ?? '',
          dataset_id: action.dataset_id,
        },
      });
      // A rejected save answers with a non-zero code and its reason has already
      // been shown by the response interceptor. Continuing would run the action
      // with the previous wiring and report a second, unrelated error — which is
      // how one click used to look like two bugs.
      const refusal = saved as { code?: number } | undefined;
      if (typeof refusal?.code === 'number' && refusal.code !== 0) {
        return;
      }
      if (!execEnabled) {
        message.success(
          t('knowledgeCompilation.ontologyInspect.actionBindingSaved', {
            defaultValue: 'Binding saved.',
          }),
        );
        setOpen(false);
        return;
      }
      setResult(
        await execute({
          id: templateId,
          actionName: action.name,
          reason: reasonText,
        }),
      );
    } catch (error) {
      message.error((error as Error)?.message);
    }
  }, [
    endpoint,
    tool,
    parseMapping,
    reason,
    readOnly,
    riskAck,
    execEnabled,
    saveBinding,
    templateId,
    action.name,
    action.dataset_id,
    binding?.id,
    binding?.note,
    qaCallable,
    execute,
    t,
  ]);

  const handleClear = useCallback(async () => {
    if (!binding) {
      return;
    }
    try {
      await deleteBinding({
        id: templateId,
        actionName: action.name,
        bindingId: binding.id,
      });
      message.success(
        t('knowledgeCompilation.ontologyInspect.actionBindingDeleted', {
          defaultValue: 'Binding removed.',
        }),
      );
      setOpen(false);
    } catch (error) {
      message.error((error as Error)?.message);
    }
  }, [binding, deleteBinding, templateId, action.name, t]);

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger asChild>{trigger}</DialogTrigger>
      <DialogContent className="max-w-2xl max-h-[85vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>
            {binding
              ? t(
                  'knowledgeCompilation.ontologyInspect.actionEditBindingTitle',
                  {
                    defaultValue: 'Edit binding · {{name}}',
                    name: action.name,
                  },
                )
              : t('knowledgeCompilation.ontologyInspect.actionBindTitle', {
                  defaultValue: 'Bind · {{name}}',
                  name: action.name,
                })}
          </DialogTitle>
        </DialogHeader>

        <div className="flex flex-col gap-3 text-xs">
          {execEnabled ? (
            <p className="text-text-secondary">
              {t('knowledgeCompilation.ontologyInspect.actionRunTestIntro', {
                defaultValue:
                  'The tool is called for real and the attempt is audited, refusals included. The exact arguments the mapping produced come back with the result.',
              })}
            </p>
          ) : (
            <p className="rounded border border-state-warning p-2 text-state-warning">
              {t(
                'knowledgeCompilation.ontologyInspect.actionExecDisabledNotice',
                {
                  defaultValue:
                    'Execution is off for this deployment (RAGFLOW_ONTOLOGY_ACTION_EXEC). Saving writes the binding only and calls nothing. Restart the backend with RAGFLOW_ONTOLOGY_ACTION_EXEC=true to call the tool for real.',
                },
              )}
            </p>
          )}

          {/* A model can list several systems, so the address is chosen per
              binding: the model's list is offered as suggestions, and a binding
              that must reach something else can type its own. Empty follows the
              model's first entry, which is shown resolved underneath. */}
          <div className="flex flex-col gap-1">
            <Label htmlFor="run-test-server" className="text-xs">
              {t('knowledgeCompilation.ontologyInspect.actionBindingAddress', {
                defaultValue: 'MCP address',
              })}
            </Label>
            <Input
              id="run-test-server"
              list="run-test-server-options"
              value={server}
              className="h-8 font-mono text-xs"
              placeholder={knownServers[0] ?? 'http://127.0.0.1:18080/mcp'}
              onChange={(e) => setServer(e.target.value)}
            />
            <datalist id="run-test-server-options">
              {knownServers.map((item) => (
                <option key={item} value={item} />
              ))}
            </datalist>
            <span className="break-all text-text-disabled">
              {t(
                'knowledgeCompilation.ontologyInspect.actionBindingAddressResolved',
                {
                  address: endpoint || '—',
                  defaultValue: 'This action dials {{address}}',
                },
              )}
            </span>
          </div>

          <div className="flex flex-col gap-1">
            <Label htmlFor="run-test-tool" className="text-xs">
              {t('knowledgeCompilation.ontologyInspect.actionBindingTool', {
                defaultValue: 'Tool',
              })}
            </Label>
            {/* The names come from the address above rather than from memory: a
                typed name the server does not expose is refused at save time as
                "tool not found", and an empty field as "mcp_tool is required".
                When the list cannot be read the field stays typeable, so an
                unreachable server does not lock the page. */}
            {toolsLoading ? (
              <span className="text-text-disabled">
                {t(
                  'knowledgeCompilation.ontologyInspect.actionBindingToolsLoading',
                  { defaultValue: 'Reading the tools…' },
                )}
              </span>
            ) : toolsError || toolNames.length === 0 ? (
              <div className="flex flex-col gap-1">
                <Input
                  id="run-test-tool"
                  value={tool}
                  className="h-8 text-xs"
                  placeholder="get_weather"
                  onChange={(e) => setTool(e.target.value)}
                />
                {toolsError ? (
                  <span className="text-state-error break-all">
                    {(toolsError as Error).message}
                  </span>
                ) : (
                  <span className="text-text-disabled">
                    {t(
                      'knowledgeCompilation.ontologyInspect.actionBindingToolsEmpty',
                      {
                        defaultValue:
                          'This server returned no tools; type the tool name if you know it.',
                      },
                    )}
                  </span>
                )}
                {privateHostBlocked && (
                  <span className="text-state-warning break-all">
                    {t(
                      'knowledgeCompilation.ontologyInspect.actionPrivateHostHint',
                      {
                        defaultValue:
                          'This deployment refuses private addresses. Set ALLOW_ANY_HOST=true in the backend environment and restart it to reach an endpoint on your own network.',
                      },
                    )}
                  </span>
                )}
                <Button
                  variant="outline"
                  size="sm"
                  className="self-start"
                  onClick={() => refetchTools()}
                >
                  {t(
                    'knowledgeCompilation.ontologyInspect.actionBindingToolsRefresh',
                    { defaultValue: 'Retry' },
                  )}
                </Button>
              </div>
            ) : (
              <Select value={tool} onValueChange={setTool}>
                <SelectTrigger id="run-test-tool" className="h-8 text-xs">
                  <SelectValue
                    placeholder={t(
                      'knowledgeCompilation.ontologyInspect.actionBindingToolPick',
                      { defaultValue: 'Pick the tool this action calls' },
                    )}
                  />
                </SelectTrigger>
                <SelectContent>
                  {toolNames.map((name) => (
                    <SelectItem key={name} value={name} className="text-xs">
                      {name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
            <span className="text-text-disabled">
              {t('knowledgeCompilation.ontologyInspect.actionBindingToolHint', {
                defaultValue:
                  'The list comes from the address above; the tool must be exposed by that server.',
              })}
            </span>
          </div>

          <div className="flex flex-col gap-1">
            <Label htmlFor="run-test-mapping" className="text-xs">
              {t('knowledgeCompilation.ontologyInspect.actionBindingMapping', {
                defaultValue: 'Argument mapping (JSON: tool argument → source)',
              })}
            </Label>
            <Textarea
              id="run-test-mapping"
              value={mappingText}
              rows={5}
              className="font-mono text-xs"
              placeholder={'{\n  "query": "contract.preconditions"\n}'}
              onChange={(e) => setMappingText(e.target.value)}
            />
            <span className="text-text-disabled">
              {t(
                'knowledgeCompilation.ontologyInspect.actionBindingMappingHint',
                {
                  defaultValue:
                    'Sources: contract.<field>, action.name|description|type, attr.<property>, literal:<json>.',
                },
              )}
            </span>
          </div>

          <label className="flex items-start gap-2">
            <Checkbox
              checked={readOnly}
              onCheckedChange={(next) => setReadOnly(!!next)}
            />
            <span>
              {t('knowledgeCompilation.ontologyInspect.actionRunTestReadOnly', {
                defaultValue:
                  'This tool only reads: calling it cannot change anything outside RAGFlow.',
              })}
            </span>
          </label>

          <label className="flex items-start gap-2">
            <Checkbox
              checked={qaCallable}
              onCheckedChange={(next) => setQaCallable(!!next)}
            />
            <span>
              {t(
                'knowledgeCompilation.ontologyInspect.actionBindingQaCallableToggle',
                {
                  defaultValue:
                    'Let an agent call this tool while answering. It still needs a read-only tool, execution switched on, and a contract that permits it.',
                },
              )}
            </span>
          </label>

          {!readOnly && (
            <label className="flex items-start gap-2 rounded border border-state-warning p-2 text-state-warning">
              <Checkbox
                checked={riskAck}
                onCheckedChange={(next) => setRiskAck(!!next)}
              />
              <span>
                {t(
                  'knowledgeCompilation.ontologyInspect.actionBindingRiskAck',
                  {
                    defaultValue:
                      'I understand: running this tool can change a system outside RAGFlow.',
                  },
                )}
              </span>
            </label>
          )}

          {execEnabled && (
            <div className="flex flex-col gap-1">
              <Label htmlFor="run-test-reason" className="text-xs">
                {t('knowledgeCompilation.ontologyInspect.actionReason', {
                  defaultValue: 'Reason for running (required)',
                })}
              </Label>
              <Textarea
                id="run-test-reason"
                value={reason}
                rows={2}
                className="text-xs"
                onChange={(e) => setReason(e.target.value)}
              />
            </div>
          )}

          {result && (
            <div className="flex flex-col gap-1 rounded border border-border-button p-2">
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-text-secondary">
                  {t(
                    'knowledgeCompilation.ontologyInspect.actionRunTestResult',
                    {
                      defaultValue: 'Result',
                    },
                  )}
                </span>
                <Badge
                  variant="secondary"
                  className={verdictTone(result.verdict)}
                >
                  {verdictLabel(t, result.verdict)}
                </Badge>
                <span className="text-text-disabled">{result.status}</span>
                {typeof result.duration_ms === 'number' && (
                  <span className="text-text-disabled">{`${result.duration_ms} ms`}</span>
                )}
              </div>
              {/* Why it was not called: the gate that decided, in words. A
                  verdict alone left the reader with nothing to act on. */}
              {verdictReasons(t, result.reasons).length > 0 && (
                <p className="break-all text-state-warning">
                  {verdictReasons(t, result.reasons).join(' · ')}
                </p>
              )}
              {result.arguments && (
                <pre className="text-text-secondary whitespace-pre-wrap break-all">
                  {JSON.stringify(result.arguments, null, 2)}
                </pre>
              )}
              {result.error && (
                <p className="text-state-error break-all">{result.error}</p>
              )}
              {result.output && (
                <pre className="text-text-primary whitespace-pre-wrap break-all">
                  {result.output}
                </pre>
              )}
            </div>
          )}
        </div>

        <DialogFooter>
          {binding && (
            <Button
              variant="outline"
              className="mr-auto"
              loading={deleting}
              onClick={handleClear}
            >
              {t('knowledgeCompilation.ontologyInspect.actionClearBinding', {
                defaultValue: 'Clear binding',
              })}
            </Button>
          )}
          <Button variant="outline" onClick={() => setOpen(false)}>
            {t('common.cancel')}
          </Button>
          {/* A binding without a tool is refused by the server, so the button
              states that up front instead of letting the save round-trip fail. */}
          <Button
            loading={saving || running}
            disabled={!tool.trim()}
            onClick={handleRun}
          >
            {execEnabled
              ? t('knowledgeCompilation.ontologyInspect.actionSaveAndRun', {
                  defaultValue: 'Save and run',
                })
              : t('knowledgeCompilation.ontologyInspect.actionSaveBinding', {
                  defaultValue: 'Save binding',
                })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
