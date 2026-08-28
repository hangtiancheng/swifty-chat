import * as z from "zod";

import { apiUrl } from "@/env";
import useAuthStore from "@/store/auth";

export class ApiError extends Error {
  code: number;

  constructor(code: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.code = code;
  }
}

const NETWORK_ERROR_CODE = -1;
const UNAUTHORIZED = 401;
const REQUEST_TIMEOUT_MS = 15_000;
const UPLOAD_TIMEOUT_MS = 120_000;

export function errorMessage(error: unknown): string {
  if (error instanceof Error && error.message) return error.message;
  return "Unexpected error";
}

/** Every handler answers HTTP 200 and puts the real status in the body. */
const envelopeSchema = z.object({
  code: z.number(),
  message: z.string().default(""),
  data: z.unknown().optional(),
});

function withTimeout(timeoutMs: number, signal?: AbortSignal): AbortSignal {
  const timeout = AbortSignal.timeout(timeoutMs);
  return signal ? AbortSignal.any([signal, timeout]) : timeout;
}

/** The token never refreshes, so an expired one can only be resolved by re-login. */
function handleUnauthorized() {
  useAuthStore.getState().clearAuth();
  if (window.location.pathname !== "/login") {
    window.location.assign("/login");
  }
}

function authHeaders(): HeadersInit {
  const { token } = useAuthStore.getState();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

async function send(
  endpoint: string,
  init: RequestInit,
  timeoutMs: number,
  signal?: AbortSignal,
): Promise<unknown> {
  let response: Response;
  try {
    response = await fetch(apiUrl + endpoint, {
      ...init,
      signal: withTimeout(timeoutMs, signal),
    });
  } catch (error) {
    // A caller-initiated abort (react-query cancellation) must stay an abort.
    if (signal?.aborted) throw error;
    const timedOut =
      error instanceof DOMException && error.name === "TimeoutError";
    throw new ApiError(
      NETWORK_ERROR_CODE,
      timedOut ? "Request timed out" : "Network error",
    );
  }

  if (!response.ok) {
    throw new ApiError(
      response.status,
      `HTTP ${response.status}: ${response.statusText}`,
    );
  }

  const envelope = envelopeSchema.safeParse(await response.json());
  if (!envelope.success) {
    throw new ApiError(
      NETWORK_ERROR_CODE,
      `Malformed response from ${endpoint}`,
    );
  }

  if (envelope.data.code === UNAUTHORIZED) {
    handleUnauthorized();
  }
  if (envelope.data.code !== 200) {
    throw new ApiError(envelope.data.code, envelope.data.message);
  }
  return envelope.data.data;
}

function parse<S extends z.ZodType>(
  endpoint: string,
  schema: S,
  data: unknown,
): z.output<S> {
  const result = schema.safeParse(data);
  if (!result.success) {
    throw new ApiError(
      NETWORK_ERROR_CODE,
      `Unexpected payload from ${endpoint}: ${z.prettifyError(result.error)}`,
    );
  }
  return result.data;
}

/** POST JSON and validate the `data` field of the envelope. */
export async function post<S extends z.ZodType>(
  endpoint: string,
  schema: S,
  body?: unknown,
  signal?: AbortSignal,
): Promise<z.output<S>> {
  const data = await send(
    endpoint,
    {
      method: "POST",
      headers: { "Content-Type": "application/json", ...authHeaders() },
      body: JSON.stringify(body ?? {}),
    },
    REQUEST_TIMEOUT_MS,
    signal,
  );
  return parse(endpoint, schema, data);
}

/** POST JSON to an endpoint whose envelope carries no `data`. */
export async function postVoid(
  endpoint: string,
  body?: unknown,
  signal?: AbortSignal,
): Promise<void> {
  await send(
    endpoint,
    {
      method: "POST",
      headers: { "Content-Type": "application/json", ...authHeaders() },
      body: JSON.stringify(body ?? {}),
    },
    REQUEST_TIMEOUT_MS,
    signal,
  );
}

/** POST multipart form data; the browser sets the boundary header itself. */
export async function upload<S extends z.ZodType>(
  endpoint: string,
  schema: S,
  form: FormData,
  signal?: AbortSignal,
): Promise<z.output<S>> {
  const data = await send(
    endpoint,
    { method: "POST", headers: authHeaders(), body: form },
    UPLOAD_TIMEOUT_MS,
    signal,
  );
  return parse(endpoint, schema, data);
}
