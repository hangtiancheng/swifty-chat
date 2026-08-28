// A worker so hashing a multi-gigabyte file never blocks the UI thread.
//
// Web Crypto has no incremental digest, and hashing a whole 2 GB file in one
// call would need the entire thing resident as an ArrayBuffer. Instead each
// slice is digested on its own and the digests are digested again, which keeps
// exactly one slice in memory and still yields a stable content address.

// `self` is the worker scope here, not a Window. Declaring the two members
// this file uses shadows the DOM global without pulling the webworker lib into
// the whole project, and types postMessage against the response union.
declare const self: {
  postMessage: (message: HashResponse) => void;
  onmessage: ((event: MessageEvent<HashRequest>) => void) | null;
};

export interface HashRequest {
  file: File;
  chunkSize: number;
}

export type HashResponse =
  | { kind: "progress"; hashed: number; total: number }
  | { kind: "done"; hash: string }
  | { kind: "error"; message: string };

const BYTE_TO_HEX = Array.from({ length: 256 }, (_, byte) =>
  byte.toString(16).padStart(2, "0"),
);

function toHex(bytes: Uint8Array): string {
  let hex = "";
  for (const byte of bytes) hex += BYTE_TO_HEX[byte];
  return hex;
}

async function digest(data: ArrayBuffer): Promise<Uint8Array> {
  return new Uint8Array(await crypto.subtle.digest("SHA-256", data));
}

async function hashFile(file: File, chunkSize: number): Promise<string> {
  const total = Math.max(1, Math.ceil(file.size / chunkSize));
  const digests = new Uint8Array(total * 32);

  for (let index = 0; index < total; index += 1) {
    const start = index * chunkSize;
    const slice = file.slice(start, start + chunkSize);
    digests.set(await digest(await slice.arrayBuffer()), index * 32);
    self.postMessage({ kind: "progress", hashed: index + 1, total });
  }

  return toHex(await digest(digests.buffer));
}

self.onmessage = async (event: MessageEvent<HashRequest>) => {
  const { file, chunkSize } = event.data;
  try {
    self.postMessage({ kind: "done", hash: await hashFile(file, chunkSize) });
  } catch (error) {
    self.postMessage({
      kind: "error",
      message:
        error instanceof Error ? error.message : "failed to hash the file",
    });
  }
};
