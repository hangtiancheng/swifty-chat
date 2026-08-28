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

import { MessageSquare } from "lucide-react";

import { SessionSidebar } from "@/components/session-sidebar";

export default function SessionList() {
  return (
    <>
      <div className="border-border w-55 shrink-0 border-r">
        <SessionSidebar />
      </div>
      <div className="text-muted-foreground/50 flex flex-1 flex-col items-center justify-center">
        <MessageSquare size={64} strokeWidth={1.5} className="mb-4" />
        <p className="text-muted-foreground/70">
          Select a conversation to start chatting
        </p>
        <p className="text-muted-foreground/50 mt-2 text-xs">
          Press ⌘K to jump to anyone
        </p>
      </div>
    </>
  );
}
