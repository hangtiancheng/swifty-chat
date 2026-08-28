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

import { Outlet } from "react-router-dom";

import { CallDialog } from "@/components/call-dialog";
import { CommandPalette } from "@/components/command-palette";
import { NavBar } from "@/components/nav-bar";
import { Card } from "@/components/ui/card";

export function AppShell() {
  return (
    <div className="bg-background flex min-h-screen items-center justify-center p-4">
      <Card className="shadow-primary/5 h-[600px] w-[1000px] flex-row gap-0 overflow-hidden p-0 shadow-xl">
        <NavBar />
        <Outlet />
      </Card>
      <CommandPalette />
      <CallDialog />
    </div>
  );
}
