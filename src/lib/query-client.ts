import { MutationCache, QueryClient } from "@tanstack/react-query";

import { ApiError, errorMessage } from "@/service/http";
import { showToast } from "@/utils/toast";

const NETWORK_ERROR_CODE = -1;

export const queryClient = new QueryClient({
  mutationCache: new MutationCache({
    onError: (error) => showToast(errorMessage(error), "error"),
  }),
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      refetchOnWindowFocus: false,
      // A rejection the backend reported will reject again; only retry transport failures.
      retry: (failureCount, error) =>
        error instanceof ApiError && error.code !== NETWORK_ERROR_CODE
          ? false
          : failureCount < 2,
    },
  },
});
