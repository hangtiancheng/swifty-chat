/**
 * Copyright (c) 2026 hangtiancheng
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

import { memoize } from "es-toolkit";

import { staticUrl } from "@/env";

function fnv1a(str: string): number {
  let hash = 0x811c9dc5;
  for (let i = 0; i < str.length; i++) {
    hash ^= str.charCodeAt(i);
    hash = Math.imul(hash, 0x01000193);
  }
  return hash >>> 0;
}

function xorShift32(seed: number): () => number {
  let state = seed || 1;
  return () => {
    state ^= state << 13;
    state ^= state >>> 17;
    state ^= state << 5;
    state >>>= 0;
    return state / 0xffffffff;
  };
}

export const genIdenticon = memoize((seed: string): string => {
  const rand = xorShift32(fnv1a(seed));

  const GRID = 5;
  const CELL = 40;
  const MARGIN = 28;
  const SIZE = GRID * CELL + MARGIN * 2;

  const canvas = document.createElement("canvas");
  canvas.width = SIZE;
  canvas.height = SIZE;
  const ctx = canvas.getContext("2d")!;

  ctx.fillStyle = "#f0f0f0";
  ctx.fillRect(0, 0, SIZE, SIZE);

  const hue = Math.floor(rand() * 360);
  const saturation = 55 + Math.floor(rand() * 15);
  const lightness = 45 + Math.floor(rand() * 15);
  ctx.fillStyle = `hsl(${hue}, ${saturation}%, ${lightness}%)`;

  for (let col = 0; col < Math.ceil(GRID / 2); col++) {
    for (let row = 0; row < GRID; row++) {
      if (rand() >= 0.5) {
        ctx.fillRect(MARGIN + col * CELL, MARGIN + row * CELL, CELL, CELL);
        const mirrorCol = GRID - 1 - col;
        if (mirrorCol !== col) {
          ctx.fillRect(
            MARGIN + mirrorCol * CELL,
            MARGIN + row * CELL,
            CELL,
            CELL,
          );
        }
      }
    }
  }
  return canvas.toDataURL();
});

const LEGACY_DEFAULT = "https://vitejs.dev/logo.svg";

export function resolveAvatar(avatar: string, seed?: string): string {
  if (!avatar || avatar === LEGACY_DEFAULT) {
    return seed ? genIdenticon(seed) : "";
  }
  return staticUrl(avatar);
}
