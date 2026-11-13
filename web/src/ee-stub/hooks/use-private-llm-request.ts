/**
 * Enterprise seam for the private-LLM admin flags. The enterprise build
 * replaces this file with the private deployment requests; open source is not
 * a private deployment, so the admin flags are off and the default-model
 * action is a no-op. Model providers and the API page are part of the open
 * source product, so their exposure flags stay on.
 */

export const useFetchEnableAdmin = () => ({ data: false, loading: false });

export const useFetchIsAdmin = () => ({ data: false, loading: false });

export const useSetDefaultLlm = () => ({
  data: undefined,
  loading: false,
  setDefaultLlm: async (_params: {
    llm_factory: string;
    llm_name: string;
  }) => {},
});

export const useFetchExposeModelProvider = () => ({
  data: true,
  loading: false,
});

export const useFetchExposeApi = () => ({ data: true, loading: false });
