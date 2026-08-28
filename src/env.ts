import { createEnv } from "@t3-oss/env-core";
import * as z from "zod";

export const env = createEnv({
  clientPrefix: "VITE_",
  client: {
    VITE_API_URL: z.url().default("http://localhost:8000"),
    VITE_WS_URL: z.url().optional(),
  },
  runtimeEnv: import.meta.env,
  emptyStringAsUndefined: true,
});

const withoutTrailingSlash = (url: string) => url.replace(/\/+$/, "");

export const apiUrl = withoutTrailingSlash(env.VITE_API_URL);

export const wsUrl = withoutTrailingSlash(
  env.VITE_WS_URL ?? apiUrl.replace(/^http/, "ws"),
);

/** Upload endpoints answer with root-relative "/static/..." paths. */
export function staticUrl(path: string): string {
  if (!path) return "";
  if (/^(https?:|wss?:|data:|blob:)/.test(path)) return path;
  return apiUrl + (path.startsWith("/") ? path : `/${path}`);
}
