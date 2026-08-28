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

import {
  format,
  formatDistanceToNowStrict,
  isToday,
  isValid,
  isYesterday,
  parse,
} from "date-fns";

/** Go renders message timestamps as "2006-01-02 15:04:05", which is not ISO. */
const WIRE_TIMESTAMP = "yyyy-MM-dd HH:mm:ss";

function parseWireTimestamp(value: string): Date | null {
  const parsed = parse(value, WIRE_TIMESTAMP, new Date());
  return isValid(parsed) ? parsed : null;
}

/**
 * Human-readable file size used when uploading files in chat.
 * Mirrors the source project's `getFileSize`.
 */
export function getFileSize(size: number): string {
  if (size < 1024) return size + "B";
  if (size < 1024 * 1024) return (size / 1024).toFixed(2) + "KB";
  if (size < 1024 * 1024 * 1024) return (size / 1024 / 1024).toFixed(2) + "MB";
  return (size / 1024 / 1024 / 1024).toFixed(2) + "GB";
}

/** Compact size label for the dashboard cache rows. */
export function formatSize(bytes: number): string {
  if (bytes < 1024) return bytes + " B";
  return (bytes / 1024).toFixed(1) + " KB";
}

/** Cache entry expirations arrive as nanoseconds. */
export function formatExpire(nanos: number): string {
  if (nanos <= 0 || nanos >= Number.MAX_SAFE_INTEGER) return "never";
  const expiresAt = new Date(nanos / 1_000_000);
  if (expiresAt.getTime() <= Date.now()) return "expired";
  return `${formatDistanceToNowStrict(expiresAt)} left`;
}

/** Clock time beside a chat bubble. */
export function formatMessageTime(value: string): string {
  const parsed = parseWireTimestamp(value);
  return parsed ? format(parsed, "HH:mm") : value;
}

/** Day separator between chat bubbles. */
export function formatMessageDay(value: string): string {
  const parsed = parseWireTimestamp(value);
  if (!parsed) return value;
  if (isToday(parsed)) return "Today";
  if (isYesterday(parsed)) return "Yesterday";
  return format(parsed, "PPP");
}

/** Groups messages sent on the same calendar day; "" when unparseable. */
export function messageDayKey(value: string): string {
  const parsed = parseWireTimestamp(value);
  return parsed ? format(parsed, "yyyy-MM-dd") : "";
}

/** Timestamp shown on a session row in the sidebar. */
export function formatSessionTime(epochMs: number): string {
  if (!epochMs) return "";
  const date = new Date(epochMs);
  if (!isValid(date)) return "";
  if (isToday(date)) return format(date, "HH:mm");
  if (isYesterday(date)) return "Yesterday";
  return format(date, "MM/dd");
}
