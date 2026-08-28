import { Semaphore } from "es-toolkit";

import { file } from "./api";
import type { UploadedFile } from "./schemas";
import type { HashRequest, HashResponse } from "@/workers/file-hash.worker";

/** The backend rejects any chunk larger than 10 MiB. */
const CHUNK_SIZE = 5 * 1024 * 1024;
/** Matches the legacy client's ceiling; the chunked endpoints have no total cap. */
const MAX_FILE_SIZE = 2 * 1024 * 1024 * 1024;
const MAX_PARALLEL_CHUNKS = 3;
const CHUNK_ATTEMPTS = 3;
const RETRY_BASE_DELAY_MS = 1000;
/** `/file/verify` validates ext_name against this, dot excluded. */
const EXT_PATTERN = /^[a-zA-Z0-9]{1,10}$/;

export interface UploadProgress {
  /** Hashing a large file takes long enough that it needs its own bar. */
  phase: "hashing" | "uploading";
  completed: number;
  total: number;
}

function extNameOf(fileName: string): string {
  const ext = fileName.includes(".") ? (fileName.split(".").pop() ?? "") : "";
  return EXT_PATTERN.test(ext) ? ext.toLowerCase() : "bin";
}

/** Runs the digest off the main thread; see the worker for why it is chunked. */
function hashInWorker(
  source: File,
  onProgress?: (progress: UploadProgress) => void,
  signal?: AbortSignal,
): Promise<string> {
  return new Promise((resolve, reject) => {
    const worker = new Worker(
      new URL("../workers/file-hash.worker.ts", import.meta.url),
      { type: "module" },
    );

    const finish = (settle: () => void) => {
      worker.terminate();
      signal?.removeEventListener("abort", onAbort);
      settle();
    };
    function onAbort() {
      finish(() => reject(signal?.reason ?? new Error("upload aborted")));
    }

    worker.onmessage = (event: MessageEvent<HashResponse>) => {
      const message = event.data;
      if (message.kind === "progress") {
        onProgress?.({
          phase: "hashing",
          completed: message.hashed,
          total: message.total,
        });
      } else if (message.kind === "done") {
        finish(() => resolve(message.hash));
      } else {
        finish(() => reject(new Error(message.message)));
      }
    };
    worker.onerror = () =>
      finish(() => reject(new Error("failed to hash the file")));

    signal?.addEventListener("abort", onAbort, { once: true });
    const request: HashRequest = { file: source, chunkSize: CHUNK_SIZE };
    worker.postMessage(request);
  });
}

const sleep = (ms: number) =>
  new Promise<void>((resolve) => setTimeout(resolve, ms));

/** Retries a chunk on transport hiccups, backing off between attempts. */
async function withRetry(send: () => Promise<unknown>, signal?: AbortSignal) {
  for (let attempt = 1; ; attempt += 1) {
    try {
      await send();
      return;
    } catch (error) {
      if (attempt >= CHUNK_ATTEMPTS || signal?.aborted) throw error;
      await sleep(RETRY_BASE_DELAY_MS * attempt);
    }
  }
}

/** Hash-verify, upload only the chunks the server is still missing, then merge. */
export async function uploadInChunks(
  source: File,
  onProgress?: (progress: UploadProgress) => void,
  signal?: AbortSignal,
): Promise<UploadedFile> {
  if (source.size > MAX_FILE_SIZE) {
    throw new Error(`File is too large (max ${MAX_FILE_SIZE / 1024 ** 3} GB)`);
  }

  const extName = extNameOf(source.name);
  const total = Math.max(1, Math.ceil(source.size / CHUNK_SIZE));
  const fileHash = await hashInWorker(source, onProgress, signal);

  const verified = await file.verify({
    file_hash: fileHash,
    chunk_cnt: total,
    ext_name: extName,
  });

  if (verified.uploaded) {
    onProgress?.({ phase: "uploading", completed: total, total });
    return {
      url: verified.url,
      file_name: source.name,
      file_size: String(source.size),
    };
  }

  let completed = total - verified.pending_chunks.length;
  onProgress?.({ phase: "uploading", completed, total });

  const gate = new Semaphore(MAX_PARALLEL_CHUNKS);
  await Promise.all(
    verified.pending_chunks.map(async (index) => {
      await gate.acquire();
      try {
        const start = index * CHUNK_SIZE;
        await withRetry(
          () =>
            file.uploadChunk(
              { file_hash: fileHash, ext_name: extName, chunk_idx: index },
              source.slice(start, start + CHUNK_SIZE),
              signal,
            ),
          signal,
        );
        completed += 1;
        onProgress?.({ phase: "uploading", completed, total });
      } finally {
        gate.release();
      }
    }),
  );

  return file.merge({
    file_hash: fileHash,
    ext_name: extName,
    file_name: source.name,
  });
}
