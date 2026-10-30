export interface IDebugSingleRequestBody {
  component_id: string;
  params: Record<string, any>;
  include_model_inputs?: boolean;
}

export interface IAgentWebhookTraceRequest {
  since_ts: number; // From the first request for return
  webhook_id: string; // Each external request generates a unique webhook ID.
}
