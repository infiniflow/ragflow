import message from '@/components/ui/message';
import {
  IModelInputSnapshot,
  ISingleDebugCaptureData,
} from '@/interfaces/database/agent';
import { IDebugSingleRequestBody } from '@/interfaces/request/agent';
import agentService from '@/services/agent-service';
import { useMutation } from '@tanstack/react-query';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useParams } from 'react-router';

interface SingleDebugResult {
  code: number;
  message: string;
  outputs: Record<string, unknown> | null;
  modelInputs?: IModelInputSnapshot;
  captureRequested: boolean;
}

/** Keeps each test run's result and loading state owned by its active request. */
export function useDebugSingle(componentId?: string, visible = true) {
  const { id } = useParams();
  const generation = useRef(0);
  const [result, setResult] = useState<SingleDebugResult>();
  const [loading, setLoading] = useState(false);
  const { mutateAsync, reset: resetMutation } = useMutation({
    gcTime: 0,
    /** Sends the debug request without adding capture fields to ordinary runs. */
    mutationFn: async (params: IDebugSingleRequestBody) => {
      const ret = await agentService.debugSingle(
        {
          agentId: id as string,
          componentId: params.component_id,
          data: {
            params: params.params,
            ...(params.include_model_inputs === true
              ? { include_model_inputs: true }
              : {}),
          },
        },
        true,
      );
      return ret.data;
    },
  });

  /** Invalidates pending responses and clears the visible result and mutation. */
  const reset = useCallback(() => {
    generation.current += 1;
    setResult(undefined);
    setLoading(false);
    resetMutation();
  }, [resetMutation]);

  useEffect(() => {
    reset();
    return () => {
      generation.current += 1;
    };
  }, [id, componentId, visible, reset]);

  /** Publishes only the latest run, parsing its result by the requested mode. */
  const debugSingle = useCallback(
    async (params: IDebugSingleRequestBody) => {
      const request = ++generation.current;
      setResult(undefined);
      setLoading(true);
      try {
        const response = await mutateAsync(params);
        if (request !== generation.current) return;
        const captureRequested = params.include_model_inputs === true;
        // Only the request determines the response shape. Ordinary component
        // outputs may legitimately contain outputs or model_inputs keys.
        const captured = captureRequested
          ? (response.data as ISingleDebugCaptureData | null)
          : null;
        setResult({
          code: response.code,
          message: response.message,
          outputs: captureRequested
            ? (captured?.outputs ?? null)
            : response.data,
          modelInputs: captured?.model_inputs,
          captureRequested,
        });
        if (response.code !== 0) message.error(response.message);
      } catch (error) {
        if (request !== generation.current) return;
        setResult({
          code: -1,
          message: error instanceof Error ? error.message : String(error),
          outputs: null,
          captureRequested: params.include_model_inputs === true,
        });
      } finally {
        if (request === generation.current) setLoading(false);
      }
    },
    [mutateAsync],
  );

  return { data: result?.outputs, result, loading, debugSingle, reset };
}
