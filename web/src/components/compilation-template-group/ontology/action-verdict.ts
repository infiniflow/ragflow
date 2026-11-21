import { TFunction } from 'i18next';

/**
 * The server reports verdicts and their reasons as stable codes.
 *
 * They are translated here rather than on the server so each language can phrase
 * them, and an unknown code still prints something a reader can act on instead
 * of exposing the identifier alone.
 */
const PREFIX = 'knowledgeCompilation.ontologyInspect';

const VERDICT_LABELS: Record<string, { key: string; text: string }> = {
  allow: { key: `${PREFIX}.verdictAllow`, text: 'allowed' },
  confirm: { key: `${PREFIX}.verdictConfirm`, text: 'needs a confirmation' },
  manual: { key: `${PREFIX}.verdictManual`, text: 'needs a person' },
  refuse: { key: `${PREFIX}.verdictRefuse`, text: 'refused' },
};

const REASON_LABELS: Record<string, { key: string; text: string }> = {
  allowed: { key: `${PREFIX}.reasonAllowed`, text: 'nothing blocks it' },
  action_not_found: {
    key: `${PREFIX}.reasonActionNotFound`,
    text: 'the action is not in the compiled model',
  },
  template_not_published: {
    key: `${PREFIX}.reasonTemplateUnpublished`,
    text: 'the model has not been published',
  },
  unbound: {
    key: `${PREFIX}.reasonUnbound`,
    text: 'no MCP tool is bound to this action',
  },
  binding_disabled: {
    key: `${PREFIX}.reasonBindingDisabled`,
    text: 'the binding is switched off',
  },
  tool_unavailable: {
    key: `${PREFIX}.reasonToolUnavailable`,
    text: 'the tool is not on this address',
  },
  not_qa_callable: {
    key: `${PREFIX}.reasonNotQACallable`,
    text: 'the binding does not let an agent call it',
  },
  not_read_only: {
    key: `${PREFIX}.reasonNotReadOnly`,
    text: 'the binding is not marked read-only',
  },
  exec_disabled: {
    key: `${PREFIX}.reasonExecDisabled`,
    text: 'execution is off for this deployment (RAGFLOW_ONTOLOGY_ACTION_EXEC)',
  },
  authorization_missing: {
    key: `${PREFIX}.reasonAuthorizationMissing`,
    text: 'the contract does not say who authorises it',
  },
  authorization_present: {
    key: `${PREFIX}.reasonAuthorizationPresent`,
    text: 'the contract requires an authorisation',
  },
  preconditions_present: {
    key: `${PREFIX}.reasonPreconditionsPresent`,
    text: 'the contract lists preconditions',
  },
};

/** A verdict in the reader's language; an unknown one is shown as-is. */
export const verdictLabel = (t: TFunction, verdict: string) => {
  const entry = VERDICT_LABELS[verdict];
  return entry ? t(entry.key, { defaultValue: entry.text }) : verdict;
};

/** A gate reason in the reader's language; an unknown one is shown as-is. */
export const reasonLabel = (t: TFunction, code: string) => {
  const entry = REASON_LABELS[code];
  return entry ? t(entry.key, { defaultValue: entry.text }) : code;
};

/**
 * The reasons behind a verdict, de-duplicated.
 *
 * `allowed` is dropped when it stands alone with nothing else to explain; a
 * verdict that needs no action should not carry a sentence saying so.
 */
export const verdictReasons = (t: TFunction, codes: string[] | undefined) => {
  const unique = Array.from(new Set(codes ?? []));
  const blocking = unique.filter((code) => code !== 'allowed');
  return (blocking.length > 0 ? blocking : unique).map((code) =>
    reasonLabel(t, code),
  );
};
