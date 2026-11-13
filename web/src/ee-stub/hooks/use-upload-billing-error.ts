export type UploadBillingResponse = {
  code?: number;
  message?: string;
  detail?: { current?: number; limit?: number; [key: string]: any };
};

/**
 * Enterprise seam for the file-upload billing errors. Open source has no
 * billing, so every upload failure is handled by the default error path.
 */
export function useUploadBillingError() {
  const handleUploadError = (_ret?: UploadBillingResponse) => {
    void _ret;
    return false;
  };

  return { handleUploadError };
}
